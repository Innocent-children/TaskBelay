package platform

import (
	"errors"

	"golang.org/x/sys/windows"
)

func TryBuildLock(path string) (func(), error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.DELETE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_TEMPORARY|windows.FILE_FLAG_DELETE_ON_CLOSE, 0)
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = windows.CloseHandle(h) }, nil
}
