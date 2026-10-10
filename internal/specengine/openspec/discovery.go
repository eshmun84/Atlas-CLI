package openspec

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/sdd"
)

// detectRoot reports whether an OpenSpec project root is present.
func detectRoot(root string) (sdd.Presence, error) {
	ok, err := dirExistsContained(root, rootDir)
	if err != nil {
		return emptyPresence(), err
	}
	if !ok {
		return emptyPresence(), nil
	}
	return enginePresence(nil), nil
}

// discoverChanges lists active and archived OpenSpec changes.
func discoverChanges(root, projectID string) (active, archived []sdd.ChangeRef, issues []sdd.Issue, err error) {
	pres, err := detectRoot(root)
	if err != nil {
		return nil, nil, nil, err
	}
	if !pres.Present {
		return nil, nil, nil, nil
	}

	changesOK, err := dirExistsContained(root, changesDir)
	if err != nil {
		return nil, nil, []sdd.Issue{{Kind: sdd.IssueUnsafe, Message: err.Error()}}, err
	}
	if !changesOK {
		// OpenSpec root without changes/ is incomplete but valid presence.
		issues = append(issues, sdd.Issue{
			Kind:    sdd.IssueIncomplete,
			Message: "openspec root present without changes directory",
		})
		return nil, nil, issues, nil
	}

	ents, err := readDirContained(root, changesDir)
	if err != nil {
		kind := sdd.IssueMalformed
		if strings.Contains(err.Error(), "symlink") {
			kind = sdd.IssueUnsafe
		}
		return nil, nil, []sdd.Issue{{Kind: kind, Message: err.Error()}}, err
	}

	for _, e := range ents {
		name := e.Name()
		if name == archiveName {
			continue
		}
		if !e.IsDir() {
			issues = append(issues, sdd.Issue{
				Kind:    sdd.IssueMalformed,
				Message: fmt.Sprintf("non-directory entry in changes: %s", name),
			})
			continue
		}
		changeRel := filepath.ToSlash(filepath.Join(changesDir, name))
		ref, cIssues := inspectChangeDir(root, projectID, changeRel, normalizeChangeID(name), false, "")
		active = append(active, ref)
		issues = append(issues, cIssues...)
	}

	archOK, err := dirExistsContained(root, archiveDir)
	if err != nil {
		issues = append(issues, sdd.Issue{Kind: sdd.IssueUnsafe, Message: err.Error()})
		return active, archived, issues, nil
	}
	if !archOK {
		return active, archived, issues, nil
	}

	archEnts, err := readDirContained(root, archiveDir)
	if err != nil {
		kind := sdd.IssueMalformed
		if strings.Contains(err.Error(), "symlink") {
			kind = sdd.IssueUnsafe
		}
		issues = append(issues, sdd.Issue{Kind: kind, Message: err.Error()})
		return active, archived, issues, nil
	}
	for _, e := range archEnts {
		if !e.IsDir() {
			issues = append(issues, sdd.Issue{
				Kind:    sdd.IssueMalformed,
				Message: fmt.Sprintf("non-directory entry in archive: %s", e.Name()),
			})
			continue
		}
		dirName := e.Name()
		changeID, _ := parseArchiveName(dirName)
		changeRel := filepath.ToSlash(filepath.Join(archiveDir, dirName))
		ref, cIssues := inspectChangeDir(root, projectID, changeRel, changeID, true, dirName)
		archived = append(archived, ref)
		issues = append(issues, cIssues...)
	}
	return active, archived, issues, nil
}
