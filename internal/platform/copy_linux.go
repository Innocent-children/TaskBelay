package platform

import (
	"golang.org/x/sys/unix"
	"os"
)

func cloneFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	err = unix.IoctlFileClone(int(out.Fd()), int(in.Fd()))
	closeErr := out.Close()
	if err != nil {
		_ = os.Remove(target)
		return err
	}
	return closeErr
}
