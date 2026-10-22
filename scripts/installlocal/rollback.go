package main

import (
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// rollbackMirror restores the Atlas Home mirror captured before this install
// mutated it.
//
// Restored when RestoreMirrorSnapshot succeeds:
//   - state/home.yaml
//   - bundled files under assets/
//   - convenience projections (agents/, adapters/, contracts/, skills/)
//   - directories this install created, when they are empty of foreign entries
//   - Home and layout directory modes recorded in the snapshot
//
// Not restored, and not deleted by this function:
//   - projects/ and any other path outside the mirror snapshot
//   - diagnostics/
//   - timestamps, ACLs, xattrs, owner, and group
//   - foreign files that make an Atlas-created directory non-empty
//     (restore stops with a conflict and leaves them)
//
// rollbackMirror runs only before the install commit. The commit is the
// successful rename of the staged binary onto prefix/bin/atlas. After that
// rename, a failed directory fsync returns errPublishDurability and must not
// call this function: both the new binary and this Home mirror stay in place
// so a later install can reconcile them. That error does not mean the install
// is durable.
//
// A process killed after a successful mirror and before rename does not run
// this function. The previous binary remains. The next successful install
// reconciles Home by mirroring forward.
func rollbackMirror(snap home.MirrorSnapshot, result home.EnsureResult) error {
	snap.Footprint = result.Footprint
	if err := home.RestoreMirrorSnapshot(snap); err != nil {
		return fmt.Errorf("rollback Atlas Home mirror: %w", err)
	}
	return nil
}
