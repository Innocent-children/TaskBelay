package platform

import (
	"os"
	"path/filepath"
	"strings"
)

type Env struct {
	HomeDir string
	DataDir string
}

func Current() (Env, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	return Env{HomeDir: dir, DataDir: filepath.Join(dir, ".taskbelay")}, nil
}

// MovedIdentity 判断两个文件标识是否为同一 inode 在不同设备号下的结果。
// 重启或重新挂载会改变 Unix 的设备号，目录本身没有变。
func MovedIdentity(a, b string) bool {
	cut := func(id string) (string, bool) {
		rest, ok := strings.CutPrefix(id, "unix-")
		if !ok {
			return "", false
		}
		_, inode, ok := strings.Cut(rest, "-")
		return inode, ok
	}
	one, ok := cut(a)
	two, also := cut(b)
	return ok && also && a != b && one == two
}
