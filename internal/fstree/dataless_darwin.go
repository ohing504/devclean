package fstree

import (
	"io/fs"
	"syscall"

	"golang.org/x/sys/unix"
)

func isDataless(info fs.FileInfo) bool {
	sys, ok := info.Sys().(*syscall.Stat_t)
	return ok && sys.Flags&unix.SF_DATALESS != 0
}
