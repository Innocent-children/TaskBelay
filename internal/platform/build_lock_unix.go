//go:build darwin || linux

package platform

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// TryBuildLock 返回 nil 表示另一个后台进程正在建同一个索引。进程终止会释放内核锁。
func TryBuildLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, nil
		}
		return nil, err
	}
	// 打开的路径可能在取得锁之前被上一持有者撤下。此时旧 inode 的锁不再代表该名称。
	held, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(held, current) {
		f.Close()
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err == nil && current.Mode()&os.ModeSymlink != 0 {
			return nil, os.ErrInvalid
		}
		return nil, err
	}
	return func() { _ = os.Remove(path); _ = f.Close() }, nil
}
