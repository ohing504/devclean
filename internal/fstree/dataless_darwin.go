package fstree

import (
	"io/fs"
	"syscall"

	"golang.org/x/sys/unix"
)

// isDataless reports whether info is a File Provider placeholder (iCloud
// Drive, Dropbox, ...) whose content is not on this disk.
func isDataless(info fs.FileInfo) bool {
	sys, ok := info.Sys().(*syscall.Stat_t)
	return ok && sys.Flags&unix.SF_DATALESS != 0
}
