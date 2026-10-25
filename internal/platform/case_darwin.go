package platform

import "golang.org/x/sys/unix"

func DirectoryCaseRule(path string) CaseRule {
	// Darwin sys/unistd.h 的 _PC_CASE_SENSITIVE 查询实际卷属性。
	value, err := unix.Pathconf(path, 11)
	if err != nil || value < 0 {
		return CaseUnknown
	}
	if value == 0 {
		return CaseInsensitive
	}
	return CaseSensitive
}
