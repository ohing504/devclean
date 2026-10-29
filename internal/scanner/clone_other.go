//go:build !darwin

package scanner

import "io/fs"

// cloneSizingSupported is false off macOS: APFS clones are macOS-only, and
// other filesystems' shared extents (btrfs, XFS reflinks) are not read.
const cloneSizingSupported = false

func fileExtents(string, fs.FileInfo) ([]extent, bool) { return nil, false }

func fileCloneInfo(string) (cloneInfo, bool) { return cloneInfo{}, false }
