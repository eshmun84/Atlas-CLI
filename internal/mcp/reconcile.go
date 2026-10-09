package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// removeAttemptBackup removes a Home-relative attempt backup after successful
// baseline restore. Overridable in tests via export_test only.
var removeAttemptBackup = home.RemoveContainedRel

// ReconcileInput drives MCP projection reconciliation for selected adapters.
type ReconcileInput struct {
	Root       string
	HomePath   string
	ProjectID  string
	Desired    DesiredState
	Adapters   []AdapterID
	Projectors map[AdapterID]Projector
	Ownership  OwnershipDocument
	DryRun     bool
	Now        func() time.Time
}

// ReconcileResult is the outcome of planning and optional apply.
type ReconcileResult struct {
	Plans       map[AdapterID]Plan
	Ownership   OwnershipDocument
	Applied     bool
	Blocked     bool
	Errors      []string
	BackupStamp string
}

// Reconcile plans (and optionally applies) MCP projections for the given adapters.
// Atlas-managed entries are created/updated/removed; user-owned entries are preserved.
// Malformed native configs and native-key collisions block without overwrite.
// Multi-adapter apply is transactional: failure rolls back natives + ownership.
func Reconcile(in ReconcileInput) (ReconcileResult, error) {
	result := ReconcileResult{
		Plans:     map[AdapterID]Plan{},
		Ownership: cloneOwnership(in.Ownership),
	}
	if err := ValidateDesiredState(in.Desired); err != nil {
		return result, fmt.Errorf("mcp reconcile: %w", err)
	}

	desiredMat := in.Desired.SelectedMaterializable()
	selectedSet := map[AdapterID]struct{}{}
	for _, a := range in.Adapters {
		selectedSet[a] = struct{}{}
	}

	allAdapters := map[AdapterID]struct{}{}
	for _, a := range in.Adapters {
		allAdapters[a] = struct{}{}
	}
	for _, a := range in.Ownership.Adapters {
		allAdapters[a.Adapter] = struct{}{}
	}

	adapterList := make([]AdapterID, 0, len(allAdapters))
	for adapter := range allAdapters {
		adapterList = append(adapterList, adapter)
	}
	sort.Slice(adapterList, func(i, j int) bool {
		return adapterList[i] < adapterList[j]
	})

	for _, adapter := range adapterList {
		proj, ok := in.Projectors[adapter]
		if !ok || proj == nil || !proj.SupportsMCP() {
			if _, selected := selectedSet[adapter]; selected && len(desiredMat) > 0 {
				result.Blocked = true
				result.Errors = append(result.Errors, fmt.Sprintf("adapter %s does not support MCP", adapter))
			}
			continue
		}

		// OpenCode JSONC is officially supported by OpenCode, but Atlas does not
		// implement JSONC-safe merging in Slice 32 — block without creating opencode.json.
		if adapter == AdapterOpenCode {
			if blocked, msg, err := OpenCodeJSONCBlocks(in.Root); err != nil {
				return result, fmt.Errorf("mcp reconcile: inspect opencode.jsonc: %w", err)
			} else if blocked {
				result.Blocked = true
				result.Errors = append(result.Errors, msg)
				result.Plans[adapter] = Plan{
					Blocked: true,
					Actions: []PlanAction{{
						Adapter: adapter,
						Kind:    ActionBlocked,
						Reason:  msg,
					}},
				}
				continue
			}
		}

		// Symlink-escape / unsafe path blocks before any write.
		if _, err := ContainedJoin(in.Root, proj.ConfigRelPath()); err != nil {
			result.Blocked = true
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", adapter, err))
			result.Plans[adapter] = Plan{
				Blocked: true,
				Actions: []PlanAction{{
					Adapter: adapter,
					Kind:    ActionBlocked,
					Reason:  err.Error(),
				}},
			}
			continue
		}

		snap, err := proj.InspectMCPProjection(in.Root)
		if err != nil {
			return result, fmt.Errorf("mcp reconcile: inspect %s: %w", adapter, err)
		}

		owned := in.Ownership.AdapterEntries(adapter)
		_, selected := selectedSet[adapter]
		var plan Plan
		built := map[string]NativeEntry{}

		if !selected {
			if len(owned) == 0 {
				plan = Plan{}
			} else {
				plan = BuildPlan(adapter, nil, snap, owned)
			}
		} else {
			plan = BuildPlan(adapter, desiredMat, snap, owned)
			if !plan.Blocked {
				for _, def := range desiredMat {
					entry, err := proj.BuildMCPProjection(in.Root, def)
					if err != nil {
						plan.Blocked = true
						plan.Actions = append(plan.Actions, PlanAction{
							Adapter:      adapter,
							DefinitionID: def.ID,
							NativeKey:    NativeKeyFor(def),
							Kind:         ActionBlocked,
							Reason:       err.Error(),
						})
						continue
					}
					if _, exists := snap.Servers[entry.Key]; exists {
						isOwned := false
						for _, o := range owned {
							if o.NativeKey == entry.Key {
								isOwned = true
								break
							}
						}
						// Content equality never establishes ownership. Any
						// pre-existing native key that is not Atlas-owned blocks.
						if !isOwned {
							plan.Blocked = true
							plan.Actions = append(plan.Actions, PlanAction{
								Adapter:      adapter,
								DefinitionID: def.ID,
								NativeKey:    entry.Key,
								Kind:         ActionBlocked,
								Reason:       "native key exists and is not Atlas-owned; refusing adopt or overwrite",
							})
							continue
						}
					}
					if _, dup := built[entry.Key]; dup {
						plan.Blocked = true
						plan.Actions = append(plan.Actions, PlanAction{
							Adapter:      adapter,
							DefinitionID: def.ID,
							NativeKey:    entry.Key,
							Kind:         ActionBlocked,
							Reason:       "duplicate native key in build map",
						})
						continue
					}
					built[entry.Key] = entry
				}
				plan = RefinePlanWithPayloads(plan, built, snap)
			}
		}

		result.Plans[adapter] = plan
		if plan.Blocked {
			result.Blocked = true
			for _, a := range plan.Actions {
				if a.Kind == ActionBlocked {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", adapter, a.Reason))
				}
			}
		}
	}

	if result.Blocked {
		return result, fmt.Errorf("mcp reconcile blocked: %s", strings.Join(result.Errors, "; "))
	}
	if in.DryRun {
		return result, nil
	}

	// Compute next ownership without writing yet.
	nextOwnership := cloneOwnership(in.Ownership)
	needsAnyWrite := false
	for adapter, plan := range result.Plans {
		if plan.NeedsWrite() {
			needsAnyWrite = true
		}
		if _, selected := selectedSet[adapter]; selected {
			nextOwnership.SetAdapterEntries(adapter, OwnershipForDesired(desiredMat))
		} else {
			nextOwnership.ClearAdapter(adapter)
		}
	}
	ownershipChanged := !ownershipEqual(in.Ownership, nextOwnership)
	if !needsAnyWrite && !ownershipChanged {
		result.Ownership = nextOwnership
		return result, nil
	}

	tx, err := CaptureSnapshots(in.Root, in.HomePath, in.ProjectID, in.Ownership, adapterList, in.Projectors)
	if err != nil {
		return result, fmt.Errorf("mcp reconcile: snapshot: %w", err)
	}

	now := time.Now().UTC()
	if in.Now != nil {
		now = in.Now().UTC()
	}
	stamp := NewBackupStamp(now)
	result.BackupStamp = stamp

	attemptBackupRel := ""
	// Persist Home backups for files that will change (never in the workspace).
	if in.HomePath != "" && in.ProjectID != "" && needsAnyWrite {
		attemptBackupRel = filepath.ToSlash(filepath.Join("projects", in.ProjectID, DirMCP, DirMCPBackups, stamp))
		for _, fs := range tx.Files {
			plan := result.Plans[fs.Adapter]
			if plan.NeedsWrite() && fs.Exists {
				if _, err := WriteHomeBackup(in.HomePath, in.ProjectID, stamp, string(fs.Adapter), fs.Raw); err != nil {
					return result, fmt.Errorf("mcp reconcile: home backup: %w", err)
				}
			}
		}
	}

	rollback := func(cause error) (ReconcileResult, error) {
		if restoreErr := RestoreSnapshots(in.Root, in.HomePath, in.ProjectID, tx); restoreErr != nil {
			// Retain attempt backup for manual recovery when rollback fails.
			return result, fmt.Errorf("mcp reconcile: %v (rollback failed: %v)", cause, restoreErr)
		}
		if attemptBackupRel != "" {
			if cleanErr := removeAttemptBackup(in.HomePath, attemptBackupRel); cleanErr != nil {
				return result, fmt.Errorf("mcp reconcile: %w (restored native/ownership baseline; transactional backup cleanup failed: %v)", cause, cleanErr)
			}
		}
		return result, fmt.Errorf("mcp reconcile: %w (rolled back)", cause)
	}

	for _, adapter := range adapterList {
		plan, ok := result.Plans[adapter]
		if !ok || !plan.NeedsWrite() {
			continue
		}
		proj := in.Projectors[adapter]
		owned := in.Ownership.AdapterEntries(adapter)
		_, selected := selectedSet[adapter]

		baselineExisted := false
		for _, fs := range tx.Files {
			if fs.Adapter == adapter && fs.Exists {
				baselineExisted = true
				break
			}
		}
		if !selected {
			err := proj.RemoveManagedMCPProjection(in.Root, OwnedKeys(owned))
			// Capture write identity even when the projector returns an error after mutating,
			// so rollback can prove the current object is still Atlas-written.
			if ferr := recordMCPWriteFootprint(&tx, in.Root, proj.ConfigRelPath(), !baselineExisted); ferr != nil && err == nil {
				err = ferr
			}
			if err != nil {
				return rollback(fmt.Errorf("remove %s: %w", adapter, err))
			}
			result.Applied = true
			continue
		}

		var entries []NativeEntry
		var managed []string
		var remove []string
		for _, action := range plan.Actions {
			switch action.Kind {
			case ActionCreate, ActionUpdate, ActionUnchanged:
				entry, err := proj.BuildMCPProjection(in.Root, defByID(desiredMat, action.DefinitionID))
				if err != nil {
					return rollback(fmt.Errorf("build %s: %w", adapter, err))
				}
				entries = append(entries, entry)
				managed = append(managed, action.NativeKey)
			case ActionRemove:
				remove = append(remove, action.NativeKey)
				managed = append(managed, action.NativeKey)
			}
		}
		for _, e := range OwnershipForDesired(desiredMat) {
			managed = appendUnique(managed, e.NativeKey)
		}
		err := proj.ApplyMCPProjection(in.Root, entries, managed, remove)
		// Capture write identity even when the projector returns an error after mutating,
		// so rollback can prove the current object is still Atlas-written.
		if ferr := recordMCPWriteFootprint(&tx, in.Root, proj.ConfigRelPath(), !baselineExisted); ferr != nil && err == nil {
			err = ferr
		}
		if err != nil {
			return rollback(fmt.Errorf("apply %s: %w", adapter, err))
		}
		result.Applied = true
	}

	result.Ownership = nextOwnership
	if in.HomePath != "" && in.ProjectID != "" && (result.Applied || ownershipChanged) {
		if err := SaveOwnership(in.HomePath, in.ProjectID, result.Ownership); err != nil {
			return rollback(fmt.Errorf("save ownership: %w", err))
		}
		if err := recordMCPWriteFootprint(&tx, in.HomePath, OwnershipRelPath(in.ProjectID), !tx.OwnershipExists); err != nil {
			return rollback(fmt.Errorf("ownership footprint: %w", err))
		}
	}
	return result, nil
}

// recordMCPWriteFootprint captures post-write identity for a path Atlas just wrote.
func recordMCPWriteFootprint(tx *TransactionSnapshot, root, rel string, created bool) error {
	if tx == nil {
		return fmt.Errorf("mcp footprint: nil transaction")
	}
	data, err := fsafety.ReadFileContained(root, rel)
	if err != nil {
		if created && os.IsNotExist(err) {
			return nil // path absent after create-path mutation; nothing to verify on rollback remove
		}
		return err
	}
	info, err := fsafety.LstatContained(root, rel)
	if err != nil {
		return err
	}
	tx.Footprint.AddFile(fsafety.WrittenFile{
		Rel:      filepath.ToSlash(rel),
		Created:  created,
		Expected: append([]byte(nil), data...),
		Mode:     info.Mode().Perm(),
		Info:     info,
	})
	return nil
}

func defByID(defs []Definition, id string) Definition {
	for _, d := range defs {
		if d.ID == id {
			return d
		}
	}
	return Definition{ID: id}
}

func appendUnique(list []string, value string) []string {
	for _, v := range list {
		if v == value {
			return list
		}
	}
	return append(list, value)
}
