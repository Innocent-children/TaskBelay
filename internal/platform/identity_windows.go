package platform

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

func directoryHandle(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
}

func FileIdentity(path string) (string, error) {
	handle, err := directoryHandle(path)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("windows-%x-%x-%x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}

func DirectoryCaseRule(path string) CaseRule {
	handle, err := directoryHandle(path)
	if err != nil {
		return CaseUnknown
	}
	defer windows.CloseHandle(handle)
	var flags uint32
	err = windows.GetFileInformationByHandleEx(handle, windows.FileCaseSensitiveInfo, (*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)))
	if err != nil {
		return CaseUnknown
	}
	if flags&1 != 0 {
		return CaseSensitive
	}
	return CaseInsensitive
}
