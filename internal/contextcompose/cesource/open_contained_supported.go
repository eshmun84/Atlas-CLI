//go:build darwin || linux

package cesource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"golang.org/x/sys/unix"
)

// OpenStrategy identifies the pack open implementation selected by build tags.
// Darwin and Linux use descriptor-relative openat with O_NOFOLLOW so neither
// leaf nor intermediate directory components can be swapped for symlinks
// during the walk. Pre-open path checks alone are not treated as race-free.
const OpenStrategy = "openat_nofollow"

// openContainedRegular opens homePath/rel as a regular file using openat(2).
//
// Sequence:
//  1. Open Atlas Home root as a directory fd.
//  2. For each intermediate component: openat(..., O_DIRECTORY|O_NOFOLLOW).
//  3. For the leaf: openat(..., O_NOFOLLOW|O_NONBLOCK).
//  4. Caller Fstats and applies the bounded read.
//
// ContainedJoin must already have validated the relative path shape.
func openContainedRegular(homePath, rel string) (*os.File, error) {
	absRoot, err := filepath.Abs(strings.TrimSpace(homePath))
	if err != nil {
		return nil, fmt.Errorf("cesource: home root: %w", err)
	}
	comps, err := relComponents(rel)
	if err != nil {
		return nil, err
	}

	rootFD, err := unix.Open(absRoot, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	dirFD := rootFD
	defer func() {
		if dirFD >= 0 {
			_ = unix.Close(dirFD)
		}
	}()

	for i, part := range comps {
		isLast := i == len(comps)-1
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if !isLast {
			flags |= unix.O_DIRECTORY
		} else {
			// Avoid blocking forever on fifo/device leaves before Fstat rejects them.
			flags |= unix.O_NONBLOCK
		}
		nextFD, err := unix.Openat(dirFD, part, flags, 0)
		if err != nil {
			if errors.Is(err, unix.ELOOP) {
				return nil, fmt.Errorf("%w: %s", fsafety.ErrSymlink, filepath.ToSlash(filepath.Join(comps[:i+1]...)))
			}
			return nil, err
		}
		_ = unix.Close(dirFD)
		dirFD = nextFD
		if isLast {
			name := filepath.Join(append([]string{absRoot}, comps...)...)
			f := os.NewFile(uintptr(dirFD), name)
			dirFD = -1 // ownership transferred to *os.File
			return f, nil
		}
	}
	return nil, fmt.Errorf("cesource: empty pack path")
}

func relComponents(rel string) ([]string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return nil, fmt.Errorf("cesource: relative path is required")
	}
	rel = filepath.ToSlash(rel)
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return nil, fsafety.ErrAbsolutePath
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fsafety.ErrPathTraversal
	}
	var comps []string
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return nil, fsafety.ErrPathTraversal
		}
		comps = append(comps, part)
	}
	if len(comps) == 0 {
		return nil, fmt.Errorf("cesource: relative path is required")
	}
	return comps, nil
}
