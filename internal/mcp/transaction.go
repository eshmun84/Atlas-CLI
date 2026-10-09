package mcp

import (
	"fmt"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// FileSnapshot captures one on-disk native config for rollback.
type FileSnapshot struct {
	Adapter AdapterID
	RelPath string
	Exists  bool
	Raw     []byte
	Mode    os.FileMode
}

// TransactionSnapshot is the pre-apply state for MCP reconciliation.
type TransactionSnapshot struct {
	OwnershipExists bool
	OwnershipRaw    []byte
	OwnershipMode   os.FileMode
	Ownership       OwnershipDocument
	Files           []FileSnapshot
	// Footprint records successful Phase B writes for identity-aware rollback.
	Footprint fsafety.TransactionFootprint
}

// CaptureSnapshots records ownership and native config bytes/modes for each projector.
func CaptureSnapshots(root, homePath, projectID string, ownership OwnershipDocument, adapters []AdapterID, projectors map[AdapterID]Projector) (TransactionSnapshot, error) {
	snap := TransactionSnapshot{Ownership: cloneOwnership(ownership)}
	if homePath != "" && projectID != "" {
		rel := OwnershipRelPath(projectID)
		info, err := fsafety.LstatContained(homePath, rel)
		if err == nil {
			data, readErr := fsafety.ReadFileContained(homePath, rel)
			if readErr != nil {
				return TransactionSnapshot{}, fmt.Errorf("mcp snapshot ownership: %w", readErr)
			}
			snap.OwnershipExists = true
			snap.OwnershipRaw = append([]byte(nil), data...)
			snap.OwnershipMode = info.Mode().Perm()
		} else if !os.IsNotExist(err) {
			return TransactionSnapshot{}, fmt.Errorf("mcp snapshot ownership: %w", err)
		}
	}
	seen := map[AdapterID]struct{}{}
	for _, adapter := range adapters {
		seen[adapter] = struct{}{}
	}
	for adapter := range projectors {
		seen[adapter] = struct{}{}
	}
	for adapter := range seen {
		proj, ok := projectors[adapter]
		if !ok || proj == nil || !proj.SupportsMCP() {
			continue
		}
		rel := proj.ConfigRelPath()
		full, err := ContainedJoin(root, rel)
		if err != nil {
			// Symlink escape / unsafe path: treat as capture error (caller blocks).
			return TransactionSnapshot{}, fmt.Errorf("mcp snapshot %s: %w", adapter, err)
		}
		fs := FileSnapshot{Adapter: adapter, RelPath: rel}
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			fs.Exists = false
			snap.Files = append(snap.Files, fs)
			continue
		}
		if err != nil {
			return TransactionSnapshot{}, fmt.Errorf("mcp snapshot %s: %w", adapter, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return TransactionSnapshot{}, fmt.Errorf("mcp snapshot %s: refusing symlink %s", adapter, rel)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return TransactionSnapshot{}, fmt.Errorf("mcp snapshot %s: %w", adapter, err)
		}
		fs.Exists = true
		fs.Raw = append([]byte(nil), data...)
		fs.Mode = info.Mode().Perm()
		snap.Files = append(snap.Files, fs)
	}
	return snap, nil
}

// RestoreSnapshots restores native configs and ownership from a snapshot using
// fsafety.RestoreFileSnapshot (baseline + write footprint identity).
// When ownership was absent before apply, any ownership file created by the
// failed attempt is removed only when the write footprint still matches.
func RestoreSnapshots(root, homePath, projectID string, snap TransactionSnapshot) error {
	var errs []string
	for _, fs := range snap.Files {
		if _, err := ContainedJoin(root, fs.RelPath); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fs.Adapter, err))
			continue
		}
		baseline := fsafety.FileSnapshot{
			Rel:     fs.RelPath,
			Exists:  fs.Exists,
			Data:    append([]byte(nil), fs.Raw...),
			Mode:    fs.Mode,
			WasFile: true,
		}
		wrote := snap.Footprint.FileByRel(fs.RelPath)
		if err := fsafety.RestoreFileSnapshot(root, baseline, wrote, snap.Footprint.DeletedByRel(fs.RelPath)); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fs.Adapter, err))
		}
	}
	if homePath != "" && projectID != "" {
		if err := restoreOwnershipSnapshot(homePath, projectID, snap); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("mcp rollback: %s", joinErrors(errs))
}

func restoreOwnershipSnapshot(homePath, projectID string, snap TransactionSnapshot) error {
	rel := OwnershipRelPath(projectID)
	baseline := fsafety.FileSnapshot{
		Rel:     rel,
		Exists:  snap.OwnershipExists,
		Data:    append([]byte(nil), snap.OwnershipRaw...),
		Mode:    snap.OwnershipMode,
		WasFile: true,
	}
	wrote := snap.Footprint.FileByRel(rel)
	if err := fsafety.RestoreFileSnapshot(homePath, baseline, wrote, snap.Footprint.DeletedByRel(rel)); err != nil {
		return fmt.Errorf("ownership restore: %w", err)
	}
	return nil
}

func cloneOwnership(doc OwnershipDocument) OwnershipDocument {
	out := OwnershipDocument{
		SchemaVersion: doc.SchemaVersion,
		ProjectID:     doc.ProjectID,
	}
	for _, a := range doc.Adapters {
		entries := append([]OwnedEntry(nil), a.Entries...)
		out.Adapters = append(out.Adapters, AdapterOwnership{
			Adapter: a.Adapter,
			Entries: entries,
		})
	}
	return out
}

func ownershipEqual(a, b OwnershipDocument) bool {
	if a.ProjectID != b.ProjectID || a.SchemaVersion != b.SchemaVersion {
		return false
	}
	if len(a.Adapters) != len(b.Adapters) {
		return false
	}
	am := map[AdapterID][]OwnedEntry{}
	for _, x := range a.Adapters {
		am[x.Adapter] = append([]OwnedEntry(nil), x.Entries...)
	}
	for _, x := range b.Adapters {
		ae, ok := am[x.Adapter]
		if !ok || len(ae) != len(x.Entries) {
			return false
		}
		for i := range ae {
			if ae[i] != x.Entries[i] {
				return false
			}
		}
		delete(am, x.Adapter)
	}
	return len(am) == 0
}

func joinErrors(errs []string) string {
	out := ""
	for i, e := range errs {
		if i > 0 {
			out += "; "
		}
		out += e
	}
	return out
}
