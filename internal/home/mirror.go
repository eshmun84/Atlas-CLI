package home

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

// EnsureResult summarizes a mutating Atlas Home ensure/mirror operation.
type EnsureResult struct {
	HomePath  string
	Created   bool
	Mirrored  []string
	UpdatedAt string
	Footprint fsafety.TransactionFootprint
}

// Sensitive Home directory permission (machine-local Atlas-owned trees).
const DirPermHome = 0o700

// EnsureLayout creates the Atlas Home directory tree. Mutating.
// Refuses symlink roots and symlink path components.
// New and existing Atlas-owned Home roots/layout dirs are hardened to 0700.
func EnsureLayout(homePath string) error {
	_, err := EnsureLayoutCreated(homePath)
	return err
}

// EnsureLayoutCreated is EnsureLayout plus CreatedDir footprints.
func EnsureLayoutCreated(homePath string) ([]fsafety.CreatedDir, error) {
	var created []fsafety.CreatedDir
	dirs, err := fsafety.SafeMkdirAllCreated(homePath, ".", DirPermHome)
	created = append(created, dirs...)
	if err != nil {
		return created, fmt.Errorf("atlas home: create %s: %w", homePath, err)
	}
	if err := chmodRealDir(homePath, DirPermHome); err != nil {
		return created, err
	}
	for _, dir := range LayoutDirectories {
		dirs, err := fsafety.SafeMkdirAllCreated(homePath, dir, DirPermHome)
		created = append(created, dirs...)
		if err != nil {
			return created, fmt.Errorf("atlas home: create %s: %w", dir, err)
		}
		if err := chmodRealDir(filepath.Join(homePath, dir), DirPermHome); err != nil {
			return created, err
		}
	}
	return created, nil
}

func chmodRealDir(path string, perm os.FileMode) error {
	info, err := lstatFn(path)
	if err != nil {
		return fmt.Errorf("atlas home: chmod %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("atlas home: refusing chmod through symlink %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("atlas home: chmod target is not a directory: %s", path)
	}
	if err := chmodFn(path, perm); err != nil {
		return fmt.Errorf("atlas home: chmod %s: %w", path, err)
	}
	return nil
}

// EnsureAndMirror creates layout (if needed) and mirrors bundled assets into Home.
// This is the only supported mutating Home bootstrap used by init/apply and repair.
func EnsureAndMirror(now time.Time) (EnsureResult, error) {
	homePath, err := Resolve()
	if err != nil {
		return EnsureResult{}, err
	}
	// Fail closed on corrupt/unreadable state before any Home mutation.
	existing, _, err := LoadState(homePath)
	if err != nil {
		return EnsureResult{}, err
	}
	created := !Exists(homePath)
	var fp fsafety.TransactionFootprint
	fail := func(err error) (EnsureResult, error) {
		return EnsureResult{HomePath: homePath, Created: created, Footprint: fp}, err
	}
	layoutDirs, err := EnsureLayoutCreated(homePath)
	fp.MergeDirs(layoutDirs)
	if err != nil {
		return fail(err)
	}

	entries := make([]StateAssetEntry, 0, len(BundledAssets()))
	mirrored := make([]string, 0, len(BundledAssets()))
	ver := version.Version
	if ver == "" {
		ver = "0.1.0"
	}

	for _, asset := range BundledAssets() {
		data, err := assets.Content.ReadFile(asset.EmbedPath)
		if err != nil {
			return fail(fmt.Errorf("atlas home: read bundled %s: %w", asset.EmbedPath, err))
		}
		sum := sha256.Sum256(data)
		checksum := hex.EncodeToString(sum[:])
		rel := filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(asset.EmbedPath)))
		w, dirs, werr := fsafety.AtomicWriteContainedTracked(homePath, rel, data, 0o644, DirPermHome, ".atlas-write-*.tmp")
		fp.MergeDirs(dirs)
		if werr != nil {
			return fail(fmt.Errorf("atlas home: write %s: %w", asset.ID, werr))
		}
		fp.AddFile(w)
		dest := AssetHomePath(homePath, asset)
		mirrored = append(mirrored, asset.ID)

		if convenience := ConvenienceHomePath(homePath, asset); convenience != "" {
			convRel, err := filepath.Rel(homePath, convenience)
			if err != nil {
				return fail(fmt.Errorf("atlas home: convenience path %s: %w", asset.ID, err))
			}
			cw, cdirs, cerr := fsafety.AtomicWriteContainedTracked(homePath, filepath.ToSlash(convRel), data, 0o644, DirPermHome, ".atlas-write-*.tmp")
			fp.MergeDirs(cdirs)
			if cerr != nil {
				return fail(fmt.Errorf("atlas home: write convenience %s: %w", asset.ID, cerr))
			}
			fp.AddFile(cw)
		}

		entries = append(entries, StateAssetEntry{
			Family:       asset.Family,
			ID:           asset.ID,
			Version:      ver,
			Checksum:     checksum,
			Source:       "bundled",
			ResolvedPath: dest,
		})
	}

	createdAt := existing.CreatedAt
	if createdAt == "" {
		createdAt = stamp(now)
	}
	doc := StateDocument{
		SchemaVersion: StateSchemaVersion,
		HomePath:      homePath,
		CreatedAt:     createdAt,
		UpdatedAt:     stamp(now),
		Source:        "bundled",
		AtlasVersion:  ver,
		Assets:        entries,
	}
	stateRel := filepath.ToSlash(filepath.Join("state", "home.yaml"))
	stateBytes, err := marshalHomeState(doc)
	if err != nil {
		return fail(err)
	}
	sw, sdirs, serr := fsafety.AtomicWriteContainedTracked(homePath, stateRel, stateBytes, 0o600, DirPermHome, ".atlas-home-*.tmp")
	fp.MergeDirs(sdirs)
	if serr != nil {
		return fail(fmt.Errorf("atlas home: write state: %w", serr))
	}
	fp.AddFile(sw)

	return EnsureResult{
		HomePath:  homePath,
		Created:   created,
		Mirrored:  mirrored,
		UpdatedAt: doc.UpdatedAt,
		Footprint: fp,
	}, nil
}

// MirrorSnapshot captures global Atlas Home mirror state for Init rollback.
// It covers state/home.yaml, bundled asset mirrors, convenience projections,
// Home/layout modes, and nested parent dirs created by asset/convenience writes.
// Footprint is filled by EnsureAndMirror for identity-aware rollback.
type MirrorSnapshot struct {
	HomePath          string
	HomeExisted       bool
	HomeMode          os.FileMode
	HomeInfo          os.FileInfo // baseline identity when HomeExisted
	LayoutDirsExisted map[string]bool
	LayoutDirModes    map[string]os.FileMode
	LayoutDirInfos    map[string]os.FileInfo // baseline identity when layout dir existed
	NestedDirsExisted map[string]bool
	State             fsafety.FileSnapshot
	Assets            []fsafety.FileSnapshot
	Footprint         fsafety.TransactionFootprint
}

// CaptureMirrorSnapshot records global Home mirror targets before EnsureAndMirror.
// Read-only: never creates Home.
// Fail-closed: unexpected Lstat/read errors abort; only os.IsNotExist means absent.
func CaptureMirrorSnapshot(homePath string) (MirrorSnapshot, error) {
	snap := MirrorSnapshot{
		HomePath:          homePath,
		LayoutDirsExisted: map[string]bool{},
		LayoutDirModes:    map[string]os.FileMode{},
		LayoutDirInfos:    map[string]os.FileInfo{},
		NestedDirsExisted: map[string]bool{},
	}
	if homePath == "" {
		return snap, nil
	}
	existed, mode, info, err := inspectRealDir(homePath)
	if err != nil {
		return snap, fmt.Errorf("atlas home: home root: %w", err)
	}
	snap.HomeExisted = existed
	snap.HomeMode = mode
	snap.HomeInfo = info
	for _, dir := range LayoutDirectories {
		existed, mode, info, err := inspectRealDir(filepath.Join(homePath, dir))
		if err != nil {
			return snap, fmt.Errorf("atlas home: layout %s: %w", dir, err)
		}
		snap.LayoutDirsExisted[dir] = existed
		if existed {
			snap.LayoutDirModes[dir] = mode
			snap.LayoutDirInfos[dir] = info
		}
	}
	stateRel := filepath.ToSlash(filepath.Join("state", "home.yaml"))
	stateSnap, err := captureFileSnapshotFn(homePath, stateRel)
	if err != nil {
		return snap, fmt.Errorf("atlas home: state snapshot: %w", err)
	}
	snap.State = stateSnap
	recordNested := func(fileRel string) error {
		for _, dir := range parentDirRels(fileRel) {
			if _, tracked := snap.NestedDirsExisted[dir]; tracked {
				continue
			}
			if _, isLayout := snap.LayoutDirsExisted[dir]; isLayout {
				// Top-level layout dirs are tracked separately.
				continue
			}
			existed, _, _, err := inspectRealDir(filepath.Join(homePath, filepath.FromSlash(dir)))
			if err != nil {
				return fmt.Errorf("atlas home: nested dir %s: %w", dir, err)
			}
			snap.NestedDirsExisted[dir] = existed
		}
		return nil
	}
	for _, asset := range BundledAssets() {
		rel := filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(asset.EmbedPath)))
		fs, err := captureFileSnapshotFn(homePath, rel)
		if err != nil {
			return snap, fmt.Errorf("atlas home: asset snapshot %s: %w", rel, err)
		}
		snap.Assets = append(snap.Assets, fs)
		if err := recordNested(rel); err != nil {
			return snap, err
		}
		if conv := ConvenienceHomePath(homePath, asset); conv != "" {
			convRel, relErr := filepath.Rel(homePath, conv)
			if relErr != nil {
				return snap, fmt.Errorf("atlas home: convenience path %s: %w", asset.ID, relErr)
			}
			convRel = filepath.ToSlash(convRel)
			cfs, cerr := captureFileSnapshotFn(homePath, convRel)
			if cerr != nil {
				return snap, fmt.Errorf("atlas home: convenience snapshot %s: %w", convRel, cerr)
			}
			snap.Assets = append(snap.Assets, cfs)
			if err := recordNested(convRel); err != nil {
				return snap, err
			}
		}
	}
	return snap, nil
}

// parentDirRels returns ancestor directory relatives for a file relative path.
func parentDirRels(fileRel string) []string {
	fileRel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(fileRel)))
	if fileRel == "" || fileRel == "." {
		return nil
	}
	parts := strings.Split(fileRel, "/")
	if len(parts) < 2 {
		return nil
	}
	out := make([]string, 0, len(parts)-1)
	for i := 1; i < len(parts); i++ {
		out = append(out, strings.Join(parts[:i], "/"))
	}
	return out
}

// RestoreMirrorSnapshot restores global Home mirror files/state after a failed Init.
// Uses baseline file snaps plus transaction Footprint (identity-aware). Never uses
// os.RemoveAll. Exact claim: existence/type/bytes/mode/identity/tracked dirs only.
func RestoreMirrorSnapshot(snap MirrorSnapshot) error {
	if snap.HomePath == "" {
		return nil
	}
	var errs []string

	// Files first (assets + state), stable order via RestoreFileSnapshot callers.
	type fileJob struct {
		snap fsafety.FileSnapshot
	}
	var jobs []fileJob
	for _, fs := range snap.Assets {
		jobs = append(jobs, fileJob{snap: fs})
	}
	jobs = append(jobs, fileJob{snap: snap.State})
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].snap.Rel < jobs[j].snap.Rel })
	for _, job := range jobs {
		wrote := snap.Footprint.FileByRel(job.snap.Rel)
		if err := fsafety.RestoreFileSnapshot(snap.HomePath, job.snap, wrote, snap.Footprint.DeletedByRel(job.snap.Rel)); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", job.snap.Rel, err))
		}
	}

	// Created dirs from footprint (deepest-first); covers new Home / layout / nested.
	if err := fsafety.RestoreCreatedDirs(snap.HomePath, snap.Footprint); err != nil {
		errs = append(errs, err.Error())
	}

	if snap.HomeExisted {
		if snap.HomeMode != 0 {
			if err := restoreDirModeIdentified(snap.HomePath, snap.HomeInfo, snap.HomeMode); err != nil {
				errs = append(errs, err.Error())
			}
		}
		for dir, existed := range snap.LayoutDirsExisted {
			if !existed {
				continue
			}
			mode, ok := snap.LayoutDirModes[dir]
			if !ok || mode == 0 {
				continue
			}
			abs := filepath.Join(snap.HomePath, dir)
			if err := restoreDirModeIdentified(abs, snap.LayoutDirInfos[dir], mode); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("atlas home mirror restore: %s", strings.Join(errs, "; "))
}

// ReadCanonical returns trusted asset bytes for built-in materialization.
// The embedded bundled asset is always the trusted source of truth.
// When a safe Home copy exists and byte-matches the embed, Home bytes may be returned;
// when Home is missing, drifted, unreadable, or unsafe (symlink), the embed is returned.
// Never follows Home symlink parents/leaves. Home health reports drift separately.
func ReadCanonical(embedPath string) ([]byte, error) {
	embedded, err := assets.Content.ReadFile(embedPath)
	if err != nil {
		return nil, fmt.Errorf("atlas home: canonical %s: %w", embedPath, err)
	}
	homePath, err := Resolve()
	if err != nil {
		return embedded, nil
	}
	rel := filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(embedPath)))
	homeData, readErr := fsafety.ReadFileContained(homePath, rel)
	if readErr != nil {
		return embedded, nil
	}
	if bytes.Equal(homeData, embedded) {
		return homeData, nil
	}
	// Drifted Home copy must never propagate into project runtime materialization.
	return embedded, nil
}
