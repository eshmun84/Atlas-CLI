//go:build windows

package memoryhealth

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// accessCheck reports whether an impersonation token is granted access to a
// security descriptor. golang.org/x/sys/windows does not wrap AccessCheck.
var accessCheck = windows.NewLazySystemDLL("advapi32.dll").NewProc("AccessCheck")

type genericMapping struct {
	GenericRead    uint32
	GenericWrite   uint32
	GenericExecute uint32
	GenericAll     uint32
}

func pathWritablePlatform(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || windowsReparse(info) {
		return false
	}
	// Windows stores the read-only attribute separately from the DACL and
	// enforces it for files. The same attribute on a directory is not a
	// content-write denial, so only non-directories consult it.
	if !info.IsDir() && info.Mode().Perm()&0o222 == 0 {
		return false
	}
	return windowsWriteAccess(path)
}

// windowsReparse reports a symlink or any other reparse point. GetNamedSecurityInfo
// follows reparse points, so the diagnostic must reject them before that call.
// A deduplicated file is also a reparse point and is reported not writable;
// that is fail-closed for this permission check, not a statement that the
// bytes are immutable.
func windowsReparse(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	attr, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok || attr == nil {
		return false
	}
	return attr.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

// windowsWriteAccess checks FILE_WRITE_DATA through the DACL. For a directory
// that bit is FILE_ADD_FILE, which is the permission required to create a
// storage file. Mandatory integrity labels require a privilege this diagnostic
// does not request, and sharing or disk state can still reject a later write.
func windowsWriteAccess(path string) bool {
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|
			windows.GROUP_SECURITY_INFORMATION|
			windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil || sd == nil {
		return false
	}

	var primary windows.Token
	err = windows.OpenProcessToken(
		windows.CurrentProcess(),
		windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE,
		&primary,
	)
	if err != nil {
		return false
	}
	defer primary.Close()

	var impersonation windows.Token
	err = windows.DuplicateTokenEx(
		primary,
		windows.TOKEN_QUERY,
		nil,
		windows.SecurityImpersonation,
		windows.TokenImpersonation,
		&impersonation,
	)
	if err != nil {
		return false
	}
	defer impersonation.Close()

	mapping := genericMapping{
		GenericRead:    uint32(windows.FILE_GENERIC_READ),
		GenericWrite:   uint32(windows.FILE_GENERIC_WRITE),
		GenericExecute: uint32(windows.FILE_GENERIC_EXECUTE),
		GenericAll:     uint32(windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1FF),
	}
	priv := make([]byte, 256)
	granted := false
	for range 2 {
		privLen := uint32(len(priv))
		var access uint32
		var status int32
		r1, _, callErr := accessCheck.Call(
			uintptr(unsafe.Pointer(sd)),
			uintptr(impersonation),
			uintptr(windows.FILE_WRITE_DATA),
			uintptr(unsafe.Pointer(&mapping)),
			uintptr(unsafe.Pointer(&priv[0])),
			uintptr(unsafe.Pointer(&privLen)),
			uintptr(unsafe.Pointer(&access)),
			uintptr(unsafe.Pointer(&status)),
		)
		runtime.KeepAlive(sd)
		runtime.KeepAlive(impersonation)
		runtime.KeepAlive(mapping)
		runtime.KeepAlive(priv)
		if r1 != 0 {
			granted = status != 0
			break
		}
		if errors.Is(callErr, windows.ERROR_INSUFFICIENT_BUFFER) && privLen > uint32(len(priv)) && privLen <= 1<<16 {
			priv = make([]byte, privLen)
			continue
		}
		return false
	}
	return granted
}
