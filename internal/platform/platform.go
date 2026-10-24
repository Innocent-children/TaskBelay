// Package platform 放和操作系统有关的差异。macOS 和 Linux 的做法相同，写在
// platform_unix.go 里；Windows 的写在 platform_windows.go 里。
package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// Env 是本机用户的位置。
type Env struct {
	Home string // 用户目录
}

// CurrentEnv 返回当前用户的 Env。
func CurrentEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	return Env{Home: home}, nil
}

// DataDir 返回 TaskBelay 存放数据的目录：用户目录下的 .taskbelay。
// macOS 和 Linux 上是 ~/.taskbelay，Windows 上是 %USERPROFILE%\.taskbelay。
func (e Env) DataDir() string {
	return filepath.Join(e.Home, ".taskbelay")
}

// PathKey 返回比较路径时用的写法：分隔符统一成正斜杠；在不区分大小写的平台上
// （Windows、macOS）统一成小写。两个路径的 PathKey 相同，就是同一个路径。
func PathKey(path string) string {
	path = filepath.ToSlash(path)
	if caseInsensitive {
		path = strings.ToLower(path)
	}
	return path
}
