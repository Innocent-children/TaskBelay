//go:build unix

package platform

import (
	"fmt"
	"os"
	"syscall"
)

func FileIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat := info.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("unix-%x-%x", stat.Dev, stat.Ino), nil
}
