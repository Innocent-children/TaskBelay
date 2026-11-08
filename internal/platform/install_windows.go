package platform

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func UserBinDir(env Env) string {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		dir = filepath.Join(env.HomeDir, "AppData", "Local")
	}
	return filepath.Join(dir, "Programs", "TaskBelay")
}

/**
 * 从句柄立即取得卷与文件编号，允许安装器同时重命名或移除目录项。
 * 仅保存路径的 FileInfo 可能在稍后的 SameFile 比较中才去读取编号。
 */
func ExecutableFileInfo(path string) (os.FileInfo, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	defer file.Close()
	return file.Stat()
}

func retireExecutable(target string) (string, error) {
	f, err := os.CreateTemp(filepath.Dir(target), ".taskbelay-retired-*.exe")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := errors.Join(f.Close(), os.Remove(path)); err != nil {
		return "", err
	}
	if err := os.Rename(target, path); err != nil {
		return "", err
	}
	return path, nil
}

func ReplaceExecutable(staged, target string) (string, error) {
	old, err := retireExecutable(target)
	if err != nil {
		return "", err
	}
	if err := os.Rename(staged, target); err != nil {
		return old, errors.Join(err, os.Rename(old, target))
	}
	if err := os.Remove(old); err != nil {
		return old, nil
	}
	return "", nil
}

func RemoveExecutable(target string) (string, error) {
	old, err := retireExecutable(target)
	if err != nil {
		return "", err
	}
	if err := os.Remove(old); err != nil {
		return old, nil
	}
	return "", nil
}

func EnableTerminalOutput(file *os.File) (func(), error) {
	handle := windows.Handle(file.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return nil, err
	}
	return func() { _ = windows.SetConsoleMode(handle, mode) }, nil
}
