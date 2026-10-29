//go:build !darwin

package fstree

import "io/fs"

func isDataless(fs.FileInfo) bool { return false }
