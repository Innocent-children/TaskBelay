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
