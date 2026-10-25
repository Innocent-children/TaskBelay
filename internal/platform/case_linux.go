package platform

import (
	"golang.org/x/sys/unix"
	"os"
)

func DirectoryCaseRule(path string) CaseRule {
	var fs unix.Statfs_t
	if unix.Statfs(path, &fs) != nil {
		return CaseUnknown
	}
	switch fs.Type {
	case unix.EXT4_SUPER_MAGIC, unix.F2FS_SUPER_MAGIC:
		file, err := os.Open(path)
		if err != nil {
			return CaseUnknown
		}
		defer file.Close()
		flags, err := unix.IoctlGetInt(int(file.Fd()), unix.FS_IOC_GETFLAGS)
		if err != nil {
			return CaseUnknown
		}
		if flags&0x40000000 != 0 {
			return CaseInsensitive
		} // FS_CASEFOLD_FL
		return CaseSensitive
	case unix.TMPFS_MAGIC, unix.BTRFS_SUPER_MAGIC:
		return CaseSensitive
	default:
		// 网络卷及其他挂载可能有独立的大小写规则，不猜测未读取的属性。
		return CaseUnknown
	}
}
