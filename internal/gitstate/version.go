package gitstate

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// minMajor、minMinor 是 TaskBelay 需要的最低 git 版本 2.5：git rev-parse --git-path 从这个版本开始有。
const minMajor, minMinor = 2, 5

var versionNumber = regexp.MustCompile(`(\d+)\.(\d+)`)

// CheckVersion 检查本机的 git 版本够不够新，不够或者找不到 git 时返回说明原因的错误。
func CheckVersion() error {
	out, err := exec.Command("git", "version").Output()
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("找不到 git 命令，TaskBelay 需要 git %d.%d 或更高版本", minMajor, minMinor)
	}
	if err != nil {
		return fmt.Errorf("git version：%w", err)
	}
	m := versionNumber.FindStringSubmatch(string(out))
	if m == nil {
		return fmt.Errorf("看不懂 git 的版本：%s", out)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < minMajor || major == minMajor && minor < minMinor {
		return fmt.Errorf("git 版本是 %d.%d，TaskBelay 需要 %d.%d 或更高版本", major, minor, minMajor, minMinor)
	}
	return nil
}
