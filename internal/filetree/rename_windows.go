package filetree

import (
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"unsafe"
)

// Directory-relative handles preserve os.Root confinement. Zero rename flags
// request an atomic failure when the target exists, for files and directories.
func renameNoReplace(dir *os.File, from, to string) error {
	name, err := windows.NewNTUnicodeString(from)
	if err != nil {
		return err
	}
	attrs := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(dir.Fd()), ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.DELETE, &attrs, &status, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	if err != nil {
		return err.(windows.NTStatus).Errno()
	}
	// Closing the handle must not obscure the rename result.
	defer func() { _ = windows.CloseHandle(handle) }()
	target, err := windows.UTF16FromString(to)
	if err != nil {
		return err
	}
	type renameInfo struct {
		Flags  uint32
		Root   windows.Handle
		Length uint32
		Name   [256]uint16
	}
	if len(target) > 256 {
		return windows.ERROR_FILENAME_EXCED_RANGE
	}
	info := renameInfo{Root: windows.Handle(dir.Fd()), Length: uint32((len(target) - 1) * 2)}
	copy(info.Name[:], target)
	err = windows.NtSetInformationFile(handle, &status, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), windows.FileRenameInformation)
	runtime.KeepAlive(dir)
	if err != nil {
		return err.(windows.NTStatus).Errno()
	}
	return nil
}
