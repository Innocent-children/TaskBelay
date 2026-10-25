//go:build unix

package platform

import (
	"os"
	"path/filepath"
)

func UserBinDir(env Env) string { return filepath.Join(env.HomeDir, ".local", "bin") }

func ReplaceExecutable(staged, target string) (string, error) {
	return "", os.Rename(staged, target)
}

func RemoveExecutable(target string) (string, error) { return "", os.Remove(target) }

func EnableTerminalOutput(_ *os.File) (func(), error) { return func() {}, nil }
