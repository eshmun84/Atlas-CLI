package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// reconcileAfterWriteHook is an optional test seam invoked after projected
// files are written and digest-verified, before ownership save. Production nil.
var reconcileAfterWriteHook func() error

// ReconcileInput drives skill projection reconciliation.
type ReconcileInput struct {
	Root       string
	HomePath   string
	ProjectID  string
	Enabled    []Pin
	Adapters   []string
	Projectors map[adapters.ID]Projector
	Ownership  OwnershipDocument
	Catalog    []Metadata
	DryRun     bool
	// ReclaimUnowned allows overwriting existing unowned projections when the
	// caller has explicitly reset Atlas Home project ownership (Init Home reset).
	ReclaimUnowned bool
	Now            func() time.Time
}

// ReconcileResult is the outcome of planning and optional apply.
type ReconcileResult struct {
	Ownership OwnershipDocument
	Applied   bool
	Blocked   bool
	Errors    []string
	Actions   []string
	// Mutation retains projection + ownership evidence for outer rollback.
	Mutation MutationState
}

// Reconcile plans and optionally applies skill projections for selected adapters.
func Reconcile(in ReconcileInput) (ReconcileResult, error) {
	result := ReconcileResult{
		Ownership: cloneOwnership(in.Ownership),
		Mutation: MutationState{
			Root:      in.Root,
			HomePath:  in.HomePath,
			ProjectID: in.ProjectID,
		},
	}
	enabled, err := ResolvePins(in.Catalog, in.Enabled)
	if err != nil {
		return result, err
	}
	enabledByID := map[string]Metadata{}
	for _, m := range enabled {
		enabledByID[m.ID] = m
	}

	now := time.Now().UTC()
	if in.Now != nil {
		now = in.Now().UTC()
	}
	stamp := now.Format(time.RFC3339)

	selected := map[string]struct{}{}
	for _, a := range in.Adapters {
		selected[a] = struct{}{}
	}

	adapterSet := map[string]struct{}{}
	for _, a := range in.Adapters {
		adapterSet[a] = struct{}{}
	}
	for _, a := range in.Ownership.Adapters {
		adapterSet[a.Adapter] = struct{}{}
	}
	adapterList := make([]string, 0, len(adapterSet))
	for a := range adapterSet {
		adapterList = append(adapterList, a)
	}
	sort.Strings(adapterList)

	type plannedWrite struct {
		adapter string
		meta    Metadata
		rootRel string
		files   map[string][]byte
	}
	type plannedRemove struct {
		adapter string
		skillID string
		rootRel string
	}

	var writes []plannedWrite
	var removes []plannedRemove

	for _, adapter := range adapterList {
		id := adapters.ID(adapter)
		proj, ok := in.Projectors[id]
		if !ok || proj == nil || !proj.SupportsSkills() {
			if _, sel := selected[adapter]; sel && len(enabled) > 0 {
				result.Blocked = true
				result.Errors = append(result.Errors, fmt.Sprintf("adapter %s does not support Skills", adapter))
			}
			continue
		}
		_, isSelected := selected[adapter]

		owned := result.Ownership.AdapterEntries(adapter)
		ownedByID := map[string]OwnedEntry{}
		for _, e := range owned {
			ownedByID[e.SkillID] = e
		}

		var nextOwned []OwnedEntry

		if isSelected {
			for _, meta := range enabled {
				rootRel := proj.PackageRootRel(meta.ID)
				conflict, err := inspectProjectionConflict(in.Root, rootRel, ownedByID[meta.ID], in.ReclaimUnowned)
				if err != nil {
					return result, err
				}
				if conflict {
					result.Blocked = true
					result.Errors = append(result.Errors,
						fmt.Sprintf("skill projection conflict at %s (unknown/user-owned; preserved)", rootRel))
					if e, ok := ownedByID[meta.ID]; ok {
						nextOwned = append(nextOwned, e)
					}
					continue
				}
				files, err := loadPackageFiles(in.HomePath, meta)
				if err != nil {
					return result, err
				}
				writes = append(writes, plannedWrite{
					adapter: adapter,
					meta:    meta,
					rootRel: rootRel,
					files:   files,
				})
				nextOwned = append(nextOwned, OwnedEntry{
					SkillID:   meta.ID,
					Version:   meta.Version,
					Digest:    meta.Digest,
					RootRel:   rootRel,
					Source:    meta.Source,
					UpdatedAt: stamp,
				})
				result.Actions = append(result.Actions, fmt.Sprintf("project %s %s@%s -> %s", adapter, meta.ID, meta.Version, rootRel))
			}
		}

		for skillID, e := range ownedByID {
			keep := false
			if isSelected {
				if _, ok := enabledByID[skillID]; ok {
					keep = true
				}
			}
			if keep {
				continue
			}
			stillOwned, err := projectionStillOwned(in.Root, e)
			if err != nil {
				return result, err
			}
			if !stillOwned {
				result.Blocked = true
				result.Errors = append(result.Errors,
					fmt.Sprintf("skill projection changed under %s; refusing removal", e.RootRel))
				nextOwned = append(nextOwned, e)
				continue
			}
			removes = append(removes, plannedRemove{adapter: adapter, skillID: skillID, rootRel: e.RootRel})
			result.Actions = append(result.Actions, fmt.Sprintf("remove %s %s (%s)", adapter, skillID, e.RootRel))
		}

		if len(nextOwned) == 0 {
			result.Ownership.ClearAdapter(adapter)
		} else {
			sort.Slice(nextOwned, func(i, j int) bool { return nextOwned[i].SkillID < nextOwned[j].SkillID })
			result.Ownership.SetAdapterEntries(adapter, nextOwned)
		}
	}

	if result.Blocked {
		return result, nil
	}
	if in.DryRun {
		return result, nil
	}

	ownExists, ownRaw, ownMode, err := captureOwnershipBaseline(in.HomePath, in.ProjectID)
	if err != nil {
		return result, fmt.Errorf("skills reconcile: ownership baseline: %w", err)
	}
	result.Mutation.OwnershipExists = ownExists
	result.Mutation.OwnershipRaw = ownRaw
	result.Mutation.OwnershipMode = ownMode

	type writePlan struct {
		plannedWrite
		desired    map[string]struct{}
		obsolete   []fsafety.FileSnapshot
		writeBases map[string]fsafety.FileSnapshot
	}
	type removePlan struct {
		plannedRemove
		baselines []fsafety.FileSnapshot
	}

	wplans := make([]writePlan, 0, len(writes))
	for _, w := range writes {
		desired := map[string]struct{}{}
		for rel := range w.files {
			desired[rel] = struct{}{}
		}
		writeBases := map[string]fsafety.FileSnapshot{}
		keys := make([]string, 0, len(w.files))
		for k := range w.files {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, rel := range keys {
			fullRel := filepath.ToSlash(filepath.Join(w.rootRel, rel))
			snap, err := fsafety.CaptureFileSnapshot(in.Root, fullRel)
			if err != nil {
				return result, fmt.Errorf("skills reconcile: snapshot %s: %w", fullRel, err)
			}
			writeBases[rel] = snap
			mergeBaseline(&result.Mutation.ProjectBaselines, snap)
		}
		obsolete, err := captureObsoletePackageFiles(in.Root, w.rootRel, desired)
		if err != nil {
			return result, fmt.Errorf("skills reconcile: obsolete scan %s: %w", w.rootRel, err)
		}
		for _, snap := range obsolete {
			mergeBaseline(&result.Mutation.ProjectBaselines, snap)
		}
		wplans = append(wplans, writePlan{
			plannedWrite: w,
			desired:      desired,
			obsolete:     obsolete,
			writeBases:   writeBases,
		})
	}

	rplans := make([]removePlan, 0, len(removes))
	for _, rm := range removes {
		bases, err := capturePackageFiles(in.Root, rm.rootRel)
		if err != nil {
			return result, fmt.Errorf("skills reconcile: snapshot remove %s: %w", rm.rootRel, err)
		}
		for _, snap := range bases {
			mergeBaseline(&result.Mutation.ProjectBaselines, snap)
		}
		rplans = append(rplans, removePlan{plannedRemove: rm, baselines: bases})
	}

	mut := &result.Mutation
	internalRollback := func() error {
		mut.Applied = true // allow Rollback to run even mid-apply
		return mut.Rollback()
	}
	fail := func(cause error) (ReconcileResult, error) {
		if rbErr := internalRollback(); rbErr != nil {
			return result, fmt.Errorf("%w (rollback failed: %v)", cause, rbErr)
		}
		return result, fmt.Errorf("%w (rolled back)", cause)
	}

	// Whole-package removals first.
	for _, rm := range rplans {
		for _, base := range rm.baselines {
			del, err := fsafety.RemoveRegularFileTracked(in.Root, base)
			if err != nil {
				return fail(fmt.Errorf("skills reconcile: remove %s: %w", base.Rel, err))
			}
			mut.ProjectFootprint.AddDeleted(del)
		}
		_ = removeEmptyDirs(in.Root, rm.rootRel) // best-effort; not required for baseline
	}

	for _, w := range wplans {
		// Remove obsolete Atlas-owned package files before/while writing desired set.
		for _, base := range w.obsolete {
			del, err := fsafety.RemoveRegularFileTracked(in.Root, base)
			if err != nil {
				return fail(fmt.Errorf("skills reconcile: remove obsolete %s: %w", base.Rel, err))
			}
			mut.ProjectFootprint.AddDeleted(del)
		}

		keys := make([]string, 0, len(w.files))
		for k := range w.files {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, rel := range keys {
			fullRel := filepath.ToSlash(filepath.Join(w.rootRel, rel))
			wrote, dirs, err := fsafety.AtomicWriteContainedTracked(in.Root, fullRel, w.files[rel], 0o644, 0o755, ".atlas-skill-*.tmp")
			mut.ProjectFootprint.MergeDirs(dirs)
			if err != nil {
				return fail(fmt.Errorf("skills reconcile: write %s: %w", fullRel, err))
			}
			mut.ProjectFootprint.AddFile(wrote)
		}

		// Verify projected digest matches catalog before ownership save.
		abs, err := fsafety.ContainedJoin(in.Root, w.rootRel)
		if err != nil {
			return fail(fmt.Errorf("skills reconcile: verify %s: %w", w.rootRel, err))
		}
		digest, _, err := PackageDigest(abs)
		if err != nil {
			return fail(fmt.Errorf("skills reconcile: verify digest %s: %w", w.rootRel, err))
		}
		if digest != w.meta.Digest {
			return fail(fmt.Errorf("skills reconcile: projected digest mismatch for %s: got %s want %s", w.rootRel, digest, w.meta.Digest))
		}
		_ = removeEmptyDirs(in.Root, w.rootRel)
	}

	if reconcileAfterWriteHook != nil {
		if err := reconcileAfterWriteHook(); err != nil {
			return fail(err)
		}
	}

	result.Ownership.UpdatedAt = stamp
	ownFP, err := SaveOwnershipTracked(in.HomePath, in.ProjectID, result.Ownership)
	mut.HomeFootprint.MergeFootprint(ownFP)
	if err != nil {
		return fail(err)
	}
	// Ensure Created flag reflects pre-mutation absence for restore semantics.
	if !ownExists {
		if w := mut.HomeFootprint.FileByRel(OwnershipRelPath(in.ProjectID)); w != nil {
			w.Created = true
		}
	}

	mut.Applied = true
	result.Applied = true
	result.Mutation = *mut
	return result, nil
}

func cloneOwnership(in OwnershipDocument) OwnershipDocument {
	out := in
	out.Adapters = append([]AdapterOwnership(nil), in.Adapters...)
	for i := range out.Adapters {
		out.Adapters[i].Entries = append([]OwnedEntry(nil), in.Adapters[i].Entries...)
	}
	return out
}

func loadPackageFiles(homePath string, meta Metadata) (map[string][]byte, error) {
	_, err := fsafety.LstatContained(homePath, meta.CanonicalRel)
	if os.IsNotExist(err) {
		return loadEmbedPackageFiles(meta.ID, meta.Version)
	}
	if err != nil {
		return nil, fmt.Errorf("skills: load package %s: %w", meta.CanonicalRel, err)
	}
	filesList, err := listPackageFilesContained(homePath, meta.CanonicalRel)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(filesList))
	for _, rel := range filesList {
		data, err := fsafety.ReadFileContained(homePath, filepath.ToSlash(filepath.Join(meta.CanonicalRel, rel)))
		if err != nil {
			return nil, err
		}
		out[rel] = data
	}
	return FilterPackageFiles(out)
}

func loadEmbedPackageFiles(id, version string) (map[string][]byte, error) {
	body, err := ReadBundledSkillMD(id, version)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{FileSkillMD: []byte(body)}
	prefix := EmbedPackagePrefix(id, version)
	if err := fsWalkEmbed(prefix, files); err != nil {
		return nil, err
	}
	return FilterPackageFiles(files)
}

func inspectProjectionConflict(root, rootRel string, owned OwnedEntry, reclaimUnowned bool) (bool, error) {
	abs, err := fsafety.ContainedJoin(root, rootRel)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true, nil
	}
	if !info.IsDir() {
		return true, nil
	}
	if owned.SkillID == "" || owned.RootRel != rootRel {
		if reclaimUnowned {
			return false, nil
		}
		return true, nil
	}
	return false, nil
}

func projectionStillOwned(root string, e OwnedEntry) (bool, error) {
	if e.RootRel == "" {
		return false, nil
	}
	abs, err := fsafety.ContainedJoin(root, e.RootRel)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, nil
	}
	digest, _, err := PackageDigest(abs)
	if err != nil {
		return false, nil
	}
	return digest == e.Digest, nil
}

func capturePackageFiles(root, rootRel string) ([]fsafety.FileSnapshot, error) {
	abs, err := fsafety.ContainedJoin(root, rootRel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing symlink package %s", rootRel)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("refusing non-directory package %s", rootRel)
	}
	var snaps []fsafety.FileSnapshot
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		st, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink under %s", rootRel)
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		pkgRel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}
		pkgRel = filepath.ToSlash(pkgRel)
		if !IsPackageFileRel(pkgRel) {
			// Outside integrity set: do not manage/delete without ownership of digest set.
			return nil
		}
		snap, err := fsafety.CaptureFileSnapshot(root, rel)
		if err != nil {
			return err
		}
		snaps = append(snaps, snap)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Rel > snaps[j].Rel })
	return snaps, nil
}

func captureObsoletePackageFiles(root, rootRel string, desired map[string]struct{}) ([]fsafety.FileSnapshot, error) {
	existing, err := capturePackageFiles(root, rootRel)
	if err != nil {
		return nil, err
	}
	abs, err := fsafety.ContainedJoin(root, rootRel)
	if err != nil {
		return nil, err
	}
	var obsolete []fsafety.FileSnapshot
	for _, snap := range existing {
		pkgRel, err := filepath.Rel(abs, filepath.Join(root, filepath.FromSlash(snap.Rel)))
		if err != nil {
			return nil, err
		}
		pkgRel = filepath.ToSlash(pkgRel)
		if _, keep := desired[pkgRel]; keep {
			continue
		}
		obsolete = append(obsolete, snap)
	}
	return obsolete, nil
}

func removeEmptyDirs(root, rootRel string) error {
	abs, err := fsafety.ContainedJoin(root, rootRel)
	if err != nil {
		return err
	}
	var dirs []string
	_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || !d.IsDir() {
			return walkErr
		}
		dirs = append(dirs, path)
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		_ = os.Remove(d) // best-effort empty dirs only
	}
	return nil
}
