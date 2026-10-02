//go:build !darwin

package scanner

// cloneSizingSupported is false off macOS: APFS clones are macOS-only, and
// other filesystems' shared extents (btrfs, XFS reflinks) are not read.
const cloneSizingSupported = false

func fileExtents(string, int64) ([]extent, bool) { return nil, false }

func fileCloneInfo(string) (cloneInfo, bool) { return cloneInfo{}, false }
