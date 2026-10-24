// Package platform 提供和操作系统有关的位置。
package platform

import (
	"os"
	"path/filepath"
)

// DataDir 返回 TaskBelay 存放数据的目录：用户目录下的 .taskbelay。
// macOS 和 Linux 上是 ~/.taskbelay，Windows 上是 %USERPROFILE%\.taskbelay。
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".taskbelay"), nil
}
