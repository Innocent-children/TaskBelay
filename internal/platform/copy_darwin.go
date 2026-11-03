package platform

import "golang.org/x/sys/unix"

func cloneFile(source, target string) error { return unix.Clonefile(source, target, 0) }
