//go:build windows

package hookactions

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"golang.org/x/sys/windows"
)

type fileAttributeTagInfo struct {
	FileAttributes uint32
	ReparseTag     uint32
}

// openLifecycleLock opens a lock leaf without following a reparse point.
// FILE_FLAG_OPEN_REPARSE_POINT applies to the final component. Replacing a
// parent directory concurrently remains outside this defense.
func openLifecycleLock(path string) (*os.File, error) {
	return openNoFollow(path, windows.OPEN_ALWAYS)
}

// createExclusiveNoFollow creates a new file. An existing leaf, including a
// reparse point, is not opened and is not written.
func createExclusiveNoFollow(path string, _ os.FileMode) (*os.File, error) {
	return openNoFollow(path, windows.CREATE_NEW)
}

func openNoFollow(path string, disposition uint32) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(
		p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		disposition,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return nil, fmt.Errorf("%w: %s", os.ErrExist, path)
		}
		return nil, err
	}
	if err := rejectUnsafeLockHandle(h, path); err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

func rejectUnsafeLockHandle(h windows.Handle, path string) error {
	var info fileAttributeTagInfo
	err := windows.GetFileInformationByHandleEx(
		h,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		return fmt.Errorf("hookactions: lock handle attributes: %w", err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%w: %s", fsafety.ErrSymlink, path)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return fmt.Errorf("%w: %s", fsafety.ErrNotRegularFile, path)
	}
	kind, err := windows.GetFileType(h)
	if err != nil || kind != windows.FILE_TYPE_DISK {
		if err == nil {
			err = fmt.Errorf("file type %d", kind)
		}
		return fmt.Errorf("%w: %s: %v", fsafety.ErrNotRegularFile, path, err)
	}
	return nil
}
