package openspec

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"github.com/eshmun84/Atlas-CLI/internal/sdd"
)

// inspectChangeDir builds a ChangeRef for one change directory.
// Optional artifacts may be missing. Ambiguous nesting fails closed.
func inspectChangeDir(root, projectID, changeRel, changeID string, archived bool, archiveDirName string) (sdd.ChangeRef, []sdd.Issue) {
	var issues []sdd.Issue
	loc := locationActive(changeID)
	if archived {
		loc = locationArchived(archiveDirName)
	}
	ref := baseChange(projectID, changeID, loc, archived)

	if archived {
		id, at := parseArchiveName(archiveDirName)
		if id != "" {
			ref.ChangeID = id
		}
		ref.ArchivedAt = at
	}

	// Ensure the change directory itself is a real directory.
	ok, err := dirExistsContained(root, changeRel)
	if err != nil {
		ref.Lifecycle = sdd.LifecycleUnknown
		ref.Ambiguous = true
		ref.Issues = append(ref.Issues, err.Error())
		issues = append(issues, sdd.Issue{Kind: sdd.IssueUnsafe, Message: err.Error(), Change: ref.ChangeID})
		return ref, issues
	}
	if !ok {
		ref.Lifecycle = sdd.LifecycleIncomplete
		ref.Issues = append(ref.Issues, "change directory missing")
		issues = append(issues, sdd.Issue{Kind: sdd.IssueIncomplete, Message: "change directory missing", Change: ref.ChangeID})
		return ref, issues
	}

	ents, err := readDirContained(root, changeRel)
	if err != nil {
		ref.Lifecycle = sdd.LifecycleUnknown
		ref.Ambiguous = true
		ref.Issues = append(ref.Issues, err.Error())
		kind := sdd.IssueMalformed
		if strings.Contains(err.Error(), "symlink") {
			kind = sdd.IssueUnsafe
		}
		issues = append(issues, sdd.Issue{Kind: kind, Message: err.Error(), Change: ref.ChangeID})
		return ref, issues
	}

	// Nested directories that look like changes (contain proposal.md) are ambiguous.
	ambiguous := false
	for _, e := range ents {
		if !e.IsDir() || e.Name() == specsDirName {
			continue
		}
		nestedRel := filepath.ToSlash(filepath.Join(changeRel, e.Name()))
		nestedProposal := filepath.ToSlash(filepath.Join(nestedRel, proposalFile))
		hasNested, nerr := fileExistsContained(root, nestedProposal)
		if nerr != nil {
			ambiguous = true
			ref.Issues = append(ref.Issues, nerr.Error())
			issues = append(issues, sdd.Issue{Kind: sdd.IssueUnsafe, Message: nerr.Error(), Change: ref.ChangeID})
			continue
		}
		if hasNested {
			ambiguous = true
			msg := fmt.Sprintf("nested change-like directory %s", e.Name())
			ref.Issues = append(ref.Issues, msg)
			issues = append(issues, sdd.Issue{Kind: sdd.IssueAmbiguous, Message: msg, Change: ref.ChangeID})
		}
	}

	proposalRel := filepath.ToSlash(filepath.Join(changeRel, proposalFile))
	tasksRel := filepath.ToSlash(filepath.Join(changeRel, tasksFile))
	hasProposal, err := fileExistsContained(root, proposalRel)
	if err != nil {
		ref.Lifecycle = sdd.LifecycleUnknown
		ref.Ambiguous = true
		ref.Issues = append(ref.Issues, err.Error())
		issues = append(issues, sdd.Issue{Kind: sdd.IssueUnsafe, Message: err.Error(), Change: ref.ChangeID})
		return ref, issues
	}
	hasTasks, err := fileExistsContained(root, tasksRel)
	if err != nil {
		ref.Lifecycle = sdd.LifecycleUnknown
		ref.Ambiguous = true
		ref.Issues = append(ref.Issues, err.Error())
		issues = append(issues, sdd.Issue{Kind: sdd.IssueUnsafe, Message: err.Error(), Change: ref.ChangeID})
		return ref, issues
	}

	tasksComplete := false
	if hasTasks {
		complete, cerr, incomplete := tasksAppearComplete(root, tasksRel)
		if cerr != nil {
			ref.Verification = sdd.VerificationIncomplete
			ref.Issues = append(ref.Issues, cerr.Error())
			issues = append(issues, sdd.Issue{Kind: sdd.IssueIncomplete, Message: cerr.Error(), Change: ref.ChangeID})
		} else if incomplete {
			ref.Verification = sdd.VerificationIncomplete
		} else {
			tasksComplete = complete
		}
	}

	specRefs, sIssues := collectSpecRefs(root, changeRel, ref.ChangeID)
	ref.SpecRefs = specRefs
	issues = append(issues, sIssues...)

	if archived {
		ref.Lifecycle = sdd.LifecycleArchived
		ref.Archived = true
		// Never invent verification success for archives.
		if ref.Verification == "" {
			ref.Verification = sdd.VerificationUnknown
		}
		if ambiguous {
			ref.Ambiguous = true
			ref.Lifecycle = sdd.LifecycleUnknown
		}
		return ref, issues
	}

	ref.Lifecycle = mapActiveLifecycle(hasProposal, hasTasks, tasksComplete, ambiguous)
	ref.Ambiguous = ambiguous
	// Ready is not verification: keep Verification independent (Unknown unless
	// already marked incomplete by unreadable/malformed task evidence).
	if ref.Lifecycle == sdd.LifecycleReady && ref.Verification == "" {
		ref.Verification = sdd.VerificationUnknown
	}
	if ref.Lifecycle == sdd.LifecycleReady && ref.Verification == sdd.VerificationPass {
		// Fail closed: adapter must never invent verification success from readiness.
		ref.Verification = sdd.VerificationUnknown
	}
	if ref.Lifecycle == sdd.LifecycleUnknown || ref.Lifecycle == sdd.LifecycleIncomplete {
		if ambiguous {
			// already recorded
		} else if !hasProposal && !hasTasks {
			issues = append(issues, sdd.Issue{
				Kind:    sdd.IssueIncomplete,
				Message: "missing proposal and tasks",
				Change:  ref.ChangeID,
			})
		}
	}
	return ref, issues
}

// tasksAppearComplete does a minimal checkbox scan.
// Returns incomplete=true when the file is unreadable or has no task markers
// (fail-closed: do not treat empty/malformed as complete).
func tasksAppearComplete(root, tasksRel string) (complete bool, err error, incomplete bool) {
	data, err := fsafety.ReadFileContained(root, tasksRel)
	if err != nil {
		return false, err, true
	}
	text := string(data)
	hasOpen := strings.Contains(text, "- [ ]")
	hasChecked := strings.Contains(text, "- [x]") || strings.Contains(text, "- [X]")
	if !hasOpen && !hasChecked {
		return false, nil, true
	}
	if hasOpen {
		return false, nil, false
	}
	return true, nil, false
}

func collectSpecRefs(root, changeRel, changeID string) ([]sdd.SpecRef, []sdd.Issue) {
	specsRel := filepath.ToSlash(filepath.Join(changeRel, specsDirName))
	ok, err := dirExistsContained(root, specsRel)
	if err != nil {
		return nil, []sdd.Issue{{Kind: sdd.IssueUnsafe, Message: err.Error(), Change: changeID}}
	}
	if !ok {
		return nil, nil
	}
	ents, err := readDirContained(root, specsRel)
	if err != nil {
		kind := sdd.IssueMalformed
		if strings.Contains(err.Error(), "symlink") {
			kind = sdd.IssueUnsafe
		}
		return nil, []sdd.Issue{{Kind: kind, Message: err.Error(), Change: changeID}}
	}
	var refs []sdd.SpecRef
	var issues []sdd.Issue
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		domain := e.Name()
		specRel := filepath.ToSlash(filepath.Join(specsRel, domain, specFileName))
		has, herr := fileExistsContained(root, specRel)
		if herr != nil {
			issues = append(issues, sdd.Issue{Kind: sdd.IssueUnsafe, Message: herr.Error(), Change: changeID})
			continue
		}
		if !has {
			continue
		}
		refs = append(refs, sdd.SpecRef{
			Name: domain,
			Ref:  specRel,
		})
	}
	return refs, issues
}
