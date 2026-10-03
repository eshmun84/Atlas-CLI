package workspace

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GitInfo describes Git repository state discovered for a workspace.
type GitInfo struct {
	IsRepo        bool
	CurrentBranch string
	Remotes       []GitRemote
}

// GitRemote is a Git remote name/URL pair.
type GitRemote struct {
	Name string
	URL  string
}

// DiscoverGit inspects Git state for root using read-only commands only.
// Missing Git or non-repo directories are non-fatal and return an empty GitInfo.
func DiscoverGit(root string) (GitInfo, error) {
	info, _, err := discoverGit(root)
	return info, err
}

func discoverGit(root string) (GitInfo, []string, error) {
	var warnings []string

	gitPath, err := exec.LookPath("git")
	if err != nil {
		warnings = append(warnings, "git is not available on PATH")
		return GitInfo{}, warnings, nil
	}

	if !isInsideWorkTree(gitPath, root) {
		return GitInfo{}, warnings, nil
	}

	info := GitInfo{IsRepo: true}

	branch, err := runGit(gitPath, root, "branch", "--show-current")
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("unable to determine current branch: %v", err))
	} else {
		info.CurrentBranch = strings.TrimSpace(branch)
	}

	remoteOut, err := runGit(gitPath, root, "remote", "-v")
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("unable to list remotes: %v", err))
	} else {
		info.Remotes = parseRemotes(remoteOut)
	}

	return info, warnings, nil
}

func isInsideWorkTree(gitPath, root string) bool {
	out, err := runGit(gitPath, root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

func runGit(gitPath, root string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command(gitPath, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

// parseRemotes parses `git remote -v` output and deduplicates fetch/push pairs.
func parseRemotes(output string) []GitRemote {
	type key struct {
		name string
		url  string
	}
	seen := make(map[key]struct{})
	var remotes []GitRemote

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		k := key{name: fields[0], url: fields[1]}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		remotes = append(remotes, GitRemote{Name: k.name, URL: k.url})
	}

	return remotes
}
