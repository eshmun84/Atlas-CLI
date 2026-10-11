package hookproj

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// applyPhaseHook is an optional test seam invoked at named apply phases.
// Phases: before_apply, after_wrapper, after_config, before_ownership,
// after_ownership, after_config_remove, after_wrapper_remove.
var applyPhaseHook func(phase string) error

func runApplyPhase(phase string) error {
	if applyPhaseHook == nil {
		return nil
	}
	return applyPhaseHook(phase)
}

// Reconcile plans and optionally applies Atlas-owned hook projections.
func Reconcile(in ReconcileInput) (ReconcileResult, error) {
	result := ReconcileResult{
		Ownership: cloneOwnership(in.Ownership),
		Mutation: MutationState{
			Root:      in.Root,
			HomePath:  in.HomePath,
			ProjectID: in.ProjectID,
		},
	}
	ownExists, ownRaw, ownMode, err := captureOwnershipBaseline(in.HomePath, in.ProjectID)
	if err != nil {
		return result, err
	}
	result.Mutation.OwnershipExists = ownExists
	result.Mutation.OwnershipRaw = ownRaw
	result.Mutation.OwnershipMode = ownMode

	stamp := time.Now().UTC().Format(time.RFC3339)
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

	type plan struct {
		adapter      string
		proj         Projector
		writeWrapper bool
		writeConfig  bool
		remove       bool
		noop         bool
		wrapperBytes []byte
		configBytes  []byte
		nextOwned    []OwnedEntry
	}
	var plans []plan
	dirty := false

	for _, adapter := range adapterList {
		id := adapters.ID(adapter)
		proj, ok := in.Projectors[id]
		if !ok || proj == nil || !proj.SupportsHooks() {
			if _, sel := selected[adapter]; sel {
				result.Blocked = true
				result.Errors = append(result.Errors, fmt.Sprintf("adapter %s does not support Hooks", adapter))
			}
			continue
		}
		_, isSelected := selected[adapter]
		ownedWrapper, hasWrapperOwn := result.Ownership.LookupKind(adapter, KindWrapper)
		ownedConfig, hasConfigOwn := result.Ownership.LookupKind(adapter, KindConfig)

		if !isSelected {
			if !hasWrapperOwn && !hasConfigOwn {
				result.Ownership.ClearAdapter(adapter)
				continue
			}
			p := plan{adapter: adapter, proj: proj, remove: true}
			if hasWrapperOwn {
				still, err := artifactMatchesOwnership(in.Root, ownedWrapper)
				if err != nil {
					return result, err
				}
				if !still {
					result.Blocked = true
					result.Errors = append(result.Errors,
						fmt.Sprintf("hooks wrapper digest drift at %s; refusing removal", ownedWrapper.PathRel))
					keep := []OwnedEntry{ownedWrapper}
					if hasConfigOwn {
						keep = append(keep, ownedConfig)
					}
					plans = append(plans, plan{adapter: adapter, proj: proj, nextOwned: keep, noop: true})
					continue
				}
			}
			if hasConfigOwn {
				okOwn, err := configMatchesOwnership(in.Root, proj, ownedConfig)
				if err != nil {
					return result, err
				}
				if !okOwn {
					result.Blocked = true
					result.Errors = append(result.Errors,
						fmt.Sprintf("hooks config digest drift at %s; refusing removal", ownedConfig.PathRel))
					keep := []OwnedEntry{}
					if hasWrapperOwn {
						keep = append(keep, ownedWrapper)
					}
					keep = append(keep, ownedConfig)
					plans = append(plans, plan{adapter: adapter, proj: proj, nextOwned: keep, noop: true})
					continue
				}
			}
			result.Actions = append(result.Actions, fmt.Sprintf("remove hooks %s", adapter))
			dirty = true
			plans = append(plans, p)
			continue
		}

		wrapperRel := proj.WrapperRel()
		configRel := proj.HooksJSONRel()
		command := proj.ManagedCommand()
		events := proj.ManagedEvents()
		wrapperBytes := append([]byte(nil), proj.WrapperBytes()...)
		if len(wrapperBytes) == 0 {
			result.Blocked = true
			result.Errors = append(result.Errors, "hooks wrapper asset missing")
			continue
		}
		desiredWrapperDigest := DigestBytes(wrapperBytes)

		// Wrapper path safety + ownership evidence before any write.
		wrapperState, err := inspectOwnedPath(in.Root, wrapperRel, hasWrapperOwn, ownedWrapper, in.ReclaimUnowned)
		if err != nil {
			return result, err
		}
		if wrapperState.conflict {
			result.Blocked = true
			result.Errors = append(result.Errors, wrapperState.reason)
			if hasWrapperOwn || hasConfigOwn {
				keep := []OwnedEntry{}
				if hasWrapperOwn {
					keep = append(keep, ownedWrapper)
				}
				if hasConfigOwn {
					keep = append(keep, ownedConfig)
				}
				plans = append(plans, plan{adapter: adapter, proj: proj, nextOwned: keep, noop: true})
			}
			continue
		}

		rawConfig, configExists, configErr := readProjectFile(in.Root, configRel)
		if configErr != nil {
			return result, configErr
		}
		if configExists {
			info, lerr := fsafety.LstatContained(in.Root, configRel)
			if lerr != nil {
				if isUnsafePathErr(lerr) {
					result.Blocked = true
					result.Errors = append(result.Errors, fmt.Sprintf("hooks config unsafe at %s", configRel))
					continue
				}
				return result, lerr
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				result.Blocked = true
				result.Errors = append(result.Errors, fmt.Sprintf("hooks config unsafe at %s", configRel))
				continue
			}
		}
		doc, perr := parseHooksJSON(rawConfig)
		if perr != nil {
			result.Blocked = true
			result.Errors = append(result.Errors, fmt.Sprintf("hooks config malformed at %s", configRel))
			continue
		}

		// Unowned managed command → never adopt by path/content equality.
		if hasManagedCommand(doc, command) && !hasConfigOwn && !in.ReclaimUnowned {
			result.Blocked = true
			result.Errors = append(result.Errors,
				fmt.Sprintf("hooks config conflict at %s (managed command present without ownership)", configRel))
			continue
		}

		// Owned config: verify managed fingerprint before update.
		if hasConfigOwn {
			currentFP := managedFingerprint(doc, command, ownedConfig.Events)
			if len(ownedConfig.Events) == 0 {
				currentFP = managedFingerprint(doc, command, events)
			}
			if configExists && currentFP != ownedConfig.Digest {
				result.Blocked = true
				result.Errors = append(result.Errors,
					fmt.Sprintf("hooks config digest drift at %s; refusing update", configRel))
				keep := []OwnedEntry{}
				if hasWrapperOwn {
					keep = append(keep, ownedWrapper)
				}
				keep = append(keep, ownedConfig)
				plans = append(plans, plan{adapter: adapter, proj: proj, nextOwned: keep, noop: true})
				continue
			}
		}

		merged := mergeManaged(doc, command, events)
		configBytes, merr := marshalHooksJSON(merged)
		if merr != nil {
			return result, merr
		}
		desiredConfigDigest := managedFingerprint(merged, command, events)

		writeWrapper := true
		writeConfig := true
		if wrapperState.exists && wrapperState.digest == desiredWrapperDigest {
			writeWrapper = false
		}
		if configExists && desiredConfigDigest == managedFingerprint(doc, command, events) &&
			hasConfigOwn && ownedConfig.Digest == desiredConfigDigest {
			// Already merged and owned fingerprint matches desired — still rewrite
			// only when bytes differ (formatting/extras). Compare raw bytes.
			if bytesEqual(rawConfig, configBytes) {
				writeConfig = false
			}
		}
		// If owned wrapper digest matches desired and no write needed for either, noop.
		if !writeWrapper && !writeConfig && hasWrapperOwn && hasConfigOwn &&
			ownedWrapper.Digest == desiredWrapperDigest && ownedConfig.Digest == desiredConfigDigest {
			plans = append(plans, plan{
				adapter: adapter, proj: proj, noop: true,
				nextOwned: []OwnedEntry{ownedWrapper, ownedConfig},
			})
			continue
		}

		next := []OwnedEntry{
			{
				Kind:      KindWrapper,
				PathRel:   wrapperRel,
				Digest:    desiredWrapperDigest,
				Source:    SourceBundled,
				UpdatedAt: stamp,
			},
			{
				Kind:      KindConfig,
				PathRel:   configRel,
				Digest:    desiredConfigDigest,
				Events:    append([]string(nil), events...),
				Source:    SourceBundled,
				UpdatedAt: stamp,
			},
		}
		result.Actions = append(result.Actions, fmt.Sprintf("project hooks %s", adapter))
		dirty = true
		plans = append(plans, plan{
			adapter:      adapter,
			proj:         proj,
			writeWrapper: writeWrapper,
			writeConfig:  writeConfig,
			wrapperBytes: wrapperBytes,
			configBytes:  configBytes,
			nextOwned:    next,
		})
	}

	if result.Blocked {
		return result, nil
	}
	if in.DryRun {
		for _, p := range plans {
			if p.remove {
				result.Ownership.ClearAdapter(p.adapter)
				continue
			}
			if len(p.nextOwned) == 0 {
				result.Ownership.ClearAdapter(p.adapter)
			} else {
				result.Ownership.SetAdapterEntries(p.adapter, p.nextOwned)
			}
		}
		return result, nil
	}

	if !dirty {
		// Pure no-op reconcile: keep ownership as planned nextOwned when provided.
		for _, p := range plans {
			if len(p.nextOwned) > 0 {
				sort.Slice(p.nextOwned, func(i, j int) bool { return p.nextOwned[i].Kind < p.nextOwned[j].Kind })
				result.Ownership.SetAdapterEntries(p.adapter, p.nextOwned)
			}
		}
		return result, nil
	}

	fail := func(cause error) (ReconcileResult, error) {
		if rbErr := result.Mutation.Rollback(); rbErr != nil {
			return result, fmt.Errorf("%w (rollback failed: %v)", cause, rbErr)
		}
		return result, fmt.Errorf("%w (rolled back)", cause)
	}

	// Apply order: per adapter → remove (config then wrapper) OR write wrapper
	// then config → finally ownership metadata. Baselines are captured before
	// each mutation so partial failure can restore every successful write.
	if err := runApplyPhase("before_apply"); err != nil {
		return fail(err)
	}

	for _, p := range plans {
		if p.noop {
			if len(p.nextOwned) == 0 {
				result.Ownership.ClearAdapter(p.adapter)
			} else {
				sort.Slice(p.nextOwned, func(i, j int) bool { return p.nextOwned[i].Kind < p.nextOwned[j].Kind })
				result.Ownership.SetAdapterEntries(p.adapter, p.nextOwned)
			}
			continue
		}
		if p.remove {
			if err := applyRemove(in.Root, p.proj, &result.Mutation); err != nil {
				return fail(err)
			}
			result.Ownership.ClearAdapter(p.adapter)
			continue
		}
		if p.writeWrapper {
			if err := writeProjectFile(in.Root, p.proj.WrapperRel(), p.wrapperBytes, 0o755, &result.Mutation); err != nil {
				return fail(err)
			}
			if err := runApplyPhase("after_wrapper"); err != nil {
				return fail(err)
			}
		}
		if p.writeConfig {
			if err := writeProjectFile(in.Root, p.proj.HooksJSONRel(), p.configBytes, 0o644, &result.Mutation); err != nil {
				return fail(err)
			}
			if err := runApplyPhase("after_config"); err != nil {
				return fail(err)
			}
		}
		if len(p.nextOwned) == 0 {
			result.Ownership.ClearAdapter(p.adapter)
		} else {
			sort.Slice(p.nextOwned, func(i, j int) bool { return p.nextOwned[i].Kind < p.nextOwned[j].Kind })
			result.Ownership.SetAdapterEntries(p.adapter, p.nextOwned)
		}
	}

	if err := runApplyPhase("before_ownership"); err != nil {
		return fail(err)
	}

	result.Ownership.UpdatedAt = stamp
	fp, err := SaveOwnershipTracked(in.HomePath, in.ProjectID, result.Ownership)
	// Merge footprint before checking err so partial ownership writes are reversible.
	result.Mutation.HomeFootprint.MergeFootprint(fp)
	result.Mutation.noteTouch()
	if err != nil {
		return fail(err)
	}
	if err := runApplyPhase("after_ownership"); err != nil {
		return fail(err)
	}
	// Full commit only after every required write succeeds.
	result.Mutation.Applied = true
	result.Applied = true
	return result, nil
}

type pathState struct {
	exists   bool
	digest   string
	conflict bool
	reason   string
}

func inspectOwnedPath(root, rel string, owned bool, entry OwnedEntry, reclaim bool) (pathState, error) {
	info, err := fsafety.LstatContained(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return pathState{}, nil
		}
		if isUnsafePathErr(err) {
			return pathState{exists: true, conflict: true, reason: fmt.Sprintf("hooks path unsafe at %s", rel)}, nil
		}
		return pathState{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return pathState{exists: true, conflict: true, reason: fmt.Sprintf("hooks path is symlink at %s", rel)}, nil
	}
	if !info.Mode().IsRegular() {
		return pathState{exists: true, conflict: true, reason: fmt.Sprintf("hooks path is not a regular file at %s", rel)}, nil
	}
	raw, _, err := readProjectFile(root, rel)
	if err != nil {
		return pathState{}, err
	}
	dig := DigestBytes(raw)
	if !owned && !reclaim {
		return pathState{
			exists: true, digest: dig, conflict: true,
			reason: fmt.Sprintf("hooks wrapper conflict at %s (unknown/user-owned; preserved)", rel),
		}, nil
	}
	if owned && dig != entry.Digest {
		return pathState{
			exists: true, digest: dig, conflict: true,
			reason: fmt.Sprintf("hooks wrapper digest drift at %s; refusing update", rel),
		}, nil
	}
	return pathState{exists: true, digest: dig}, nil
}

func artifactMatchesOwnership(root string, e OwnedEntry) (bool, error) {
	raw, exists, err := readProjectFile(root, e.PathRel)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil // already gone — safe to clear ownership
	}
	info, err := fsafety.LstatContained(root, e.PathRel)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, nil
	}
	return DigestBytes(raw) == e.Digest, nil
}

func configMatchesOwnership(root string, proj Projector, e OwnedEntry) (bool, error) {
	raw, exists, err := readProjectFile(root, e.PathRel)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	info, err := fsafety.LstatContained(root, e.PathRel)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, nil
	}
	doc, err := parseHooksJSON(raw)
	if err != nil {
		return false, nil
	}
	events := e.Events
	if len(events) == 0 {
		events = proj.ManagedEvents()
	}
	return managedFingerprint(doc, proj.ManagedCommand(), events) == e.Digest, nil
}

func applyRemove(root string, proj Projector, mut *MutationState) error {
	configRel := proj.HooksJSONRel()
	raw, exists, err := readProjectFile(root, configRel)
	if err != nil {
		return err
	}
	if exists {
		doc, perr := parseHooksJSON(raw)
		if perr != nil {
			return perr
		}
		cleaned := removeManaged(doc, proj.ManagedCommand())
		if isEmptyHooks(cleaned) && len(cleaned.Extra) == 0 {
			if err := removeProjectFile(root, configRel, mut); err != nil {
				return err
			}
		} else {
			data, merr := marshalHooksJSON(cleaned)
			if merr != nil {
				return merr
			}
			if err := writeProjectFile(root, configRel, data, 0o644, mut); err != nil {
				return err
			}
		}
		if err := runApplyPhase("after_config_remove"); err != nil {
			return err
		}
	}
	if err := removeProjectFile(root, proj.WrapperRel(), mut); err != nil {
		return err
	}
	return runApplyPhase("after_wrapper_remove")
}

func readProjectFile(root, rel string) ([]byte, bool, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	data, err := fsafety.ReadFileContained(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func writeProjectFile(root, rel string, data []byte, mode os.FileMode, mut *MutationState) error {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	snap, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil {
		return err
	}
	mergeBaseline(&mut.ProjectBaselines, snap)
	mut.noteTouch()
	parent := filepath.ToSlash(filepath.Dir(rel))
	if parent != "" && parent != "." {
		dirs, err := fsafety.SafeMkdirAllCreated(root, parent, 0o755)
		mut.ProjectFootprint.MergeDirs(dirs)
		if err != nil {
			return err
		}
	}
	wrote, writeDirs, err := fsafety.AtomicWriteContainedTracked(root, rel, data, mode, 0o755, ".atlas-hooks-*.tmp")
	mut.ProjectFootprint.MergeDirs(writeDirs)
	if err != nil {
		return err
	}
	mut.ProjectFootprint.AddFile(wrote)
	return nil
}

func removeProjectFile(root, rel string, mut *MutationState) error {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	snap, err := fsafety.CaptureFileSnapshot(root, rel)
	if err != nil {
		return err
	}
	if !snap.Exists || !snap.WasFile {
		return nil
	}
	mergeBaseline(&mut.ProjectBaselines, snap)
	mut.noteTouch()
	deleted, err := fsafety.RemoveRegularFileTracked(root, snap)
	if err != nil {
		return err
	}
	mut.ProjectFootprint.AddDeleted(deleted)
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isUnsafePathErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, fsafety.ErrSymlink) ||
		errors.Is(err, fsafety.ErrPathTraversal) ||
		errors.Is(err, fsafety.ErrPathEscape) ||
		errors.Is(err, fsafety.ErrAbsolutePath) ||
		errors.Is(err, fsafety.ErrNotRegularFile)
}
