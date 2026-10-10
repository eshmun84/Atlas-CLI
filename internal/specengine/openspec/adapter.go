package openspec

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/sdd"
)

// Adapter is the read-only OpenSpec Spec Engine implementation.
// It maps OpenSpec filesystem layout onto neutral sdd types without copying
// proposal/spec/design/tasks/verify/archive contents into Atlas storage.
type Adapter struct{}

// New returns an OpenSpec Spec Engine adapter.
func New() *Adapter { return &Adapter{} }

// ID implements sdd.Engine.
func (a *Adapter) ID() sdd.EngineID { return sdd.EngineID(engineID) }

// Detect implements sdd.Engine.
func (a *Adapter) Detect(req sdd.InspectRequest) (sdd.Presence, error) {
	root := strings.TrimSpace(req.Root)
	if root == "" {
		return emptyPresence(), fmt.Errorf("openspec: root is required")
	}
	return detectRoot(root)
}

// ListChanges implements sdd.Engine.
func (a *Adapter) ListChanges(req sdd.InspectRequest) (active, archived []sdd.ChangeRef, issues []sdd.Issue, err error) {
	root := strings.TrimSpace(req.Root)
	if root == "" {
		return nil, nil, nil, fmt.Errorf("openspec: root is required")
	}
	return discoverChanges(root, strings.TrimSpace(req.ProjectID))
}

// InspectChange implements sdd.Engine.
func (a *Adapter) InspectChange(req sdd.InspectRequest, changeID string) (sdd.ChangeRef, error) {
	root := strings.TrimSpace(req.Root)
	changeID = normalizeChangeID(changeID)
	if root == "" {
		return sdd.ChangeRef{}, fmt.Errorf("openspec: root is required")
	}
	if changeID == "" {
		return sdd.ChangeRef{}, fmt.Errorf("openspec: change id is required")
	}
	if !validChangeID(changeID) {
		return sdd.ChangeRef{}, fmt.Errorf("openspec: path traversal refused in change id")
	}
	projectID := strings.TrimSpace(req.ProjectID)

	activeRel := locationActive(changeID)
	ok, err := dirExistsContained(root, activeRel)
	if err != nil {
		return sdd.ChangeRef{}, err
	}
	if ok {
		ref, _ := inspectChangeDir(root, projectID, activeRel, changeID, false, "")
		return ref, nil
	}

	// Search archive by suffix match on change id.
	archOK, err := dirExistsContained(root, archiveDir)
	if err != nil {
		return sdd.ChangeRef{}, err
	}
	if !archOK {
		return sdd.ChangeRef{}, fmt.Errorf("openspec: change %q not found", changeID)
	}
	ents, err := readDirContained(root, archiveDir)
	if err != nil {
		return sdd.ChangeRef{}, err
	}
	var match string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		id, _ := parseArchiveName(e.Name())
		if id == changeID || e.Name() == changeID {
			if match != "" {
				return sdd.ChangeRef{
					ProjectID: projectID,
					Engine:    sdd.EngineID(engineID),
					ChangeID:  changeID,
					Lifecycle: sdd.LifecycleUnknown,
					Ambiguous: true,
					Issues:    []string{"multiple archive matches"},
				}, fmt.Errorf("openspec: ambiguous archive match for %q", changeID)
			}
			match = e.Name()
		}
	}
	if match == "" {
		return sdd.ChangeRef{}, fmt.Errorf("openspec: change %q not found", changeID)
	}
	changeRel := locationArchived(match)
	ref, _ := inspectChangeDir(root, projectID, changeRel, changeID, true, match)
	return ref, nil
}

// Ensure Adapter satisfies sdd.Engine at compile time.
var _ sdd.Engine = (*Adapter)(nil)
