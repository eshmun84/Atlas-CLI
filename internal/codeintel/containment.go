package codeintel

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Known CodeGraph side-effect under the product repo (not Atlas Home).
const knownJournalRel = ".codegraph/changes.journal"

// ContainmentReport describes side-effect cleanup after a refresh.
type ContainmentReport struct {
	PreexistingDir bool
	JournalRemoved bool
	DirRemoved     bool
	Preserved      []string
	Warnings       []string
}

type containmentSnapshot struct {
	dirExisted     bool
	journalExisted bool
	journalMod     time.Time
	journalSize    int64
	otherFiles     []string
}

func snapshotCodegraphSideEffects(root string) containmentSnapshot {
	var snap containmentSnapshot
	dir := filepath.Join(root, ".codegraph")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return snap
	}
	snap.dirExisted = true
	_ = filepath.Walk(dir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil || fi == nil || fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel == knownJournalRel {
			snap.journalExisted = true
			snap.journalMod = fi.ModTime()
			snap.journalSize = fi.Size()
			return nil
		}
		snap.otherFiles = append(snap.otherFiles, rel)
		return nil
	})
	return snap
}

// containCodegraphSideEffects removes only the known journal when safe.
func containCodegraphSideEffects(root string, before containmentSnapshot, startedAt time.Time) ContainmentReport {
	report := ContainmentReport{
		PreexistingDir: before.dirExisted,
		Preserved:      append([]string(nil), before.otherFiles...),
	}
	journalPath := filepath.Join(root, filepath.FromSlash(knownJournalRel))
	info, err := os.Stat(journalPath)
	if err != nil {
		if !os.IsNotExist(err) {
			report.Warnings = append(report.Warnings, "containment: cannot stat journal: "+err.Error())
		}
		return report
	}

	// Unknown new files under .codegraph → warn, never delete.
	afterOthers := listOtherCodegraphFiles(root)
	for _, rel := range afterOthers {
		known := false
		for _, prev := range before.otherFiles {
			if prev == rel {
				known = true
				break
			}
		}
		if !known {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("containment: leaving unknown path %s untouched", rel))
			report.Preserved = append(report.Preserved, rel)
		}
	}

	canRemoveJournal := false
	switch {
	case !before.journalExisted:
		// Created during this invocation.
		canRemoveJournal = true
	case before.journalExisted:
		// Preexisting journal: only remove if clearly rewritten after start AND
		// we still refuse when prior content ownership is ambiguous — preserve.
		_ = info
		_ = startedAt
		report.Warnings = append(report.Warnings,
			"containment: preexisting .codegraph/changes.journal preserved")
		canRemoveJournal = false
	}

	if !canRemoveJournal {
		return report
	}

	if err := os.Remove(journalPath); err != nil {
		report.Warnings = append(report.Warnings, "containment: remove journal: "+err.Error())
		return report
	}
	report.JournalRemoved = true

	if !before.dirExisted {
		dir := filepath.Join(root, ".codegraph")
		entries, readErr := os.ReadDir(dir)
		if readErr == nil && len(entries) == 0 {
			if err := os.Remove(dir); err != nil {
				report.Warnings = append(report.Warnings, "containment: remove empty .codegraph: "+err.Error())
			} else {
				report.DirRemoved = true
			}
		}
	}
	return report
}

func listOtherCodegraphFiles(root string) []string {
	var out []string
	dir := filepath.Join(root, ".codegraph")
	_ = filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil || fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel != knownJournalRel {
			out = append(out, rel)
		}
		return nil
	})
	return out
}
