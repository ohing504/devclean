package scanner

import (
	"encoding/binary"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	cloneSizingSupported = true
	apfsBlock            = 4096
)

// fileExtents returns a file's physical extents via fcntl(F_LOG2PHYS_EXT). ok
// is false when a lookup fails (e.g. a compressed file).
func fileExtents(path string, size int64) (xs []extent, ok bool) {
	if size == 0 {
		return nil, true
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, false
	}
	defer func() { _ = unix.Close(fd) }()

	// struct log2phys, pack(4): u32 flags, off_t contigbytes, off_t devoffset.
	var l [20]byte
	for off := int64(0); off < size; {
		binary.LittleEndian.PutUint32(l[0:], 0)
		binary.LittleEndian.PutUint64(l[4:], uint64(size-off))
		binary.LittleEndian.PutUint64(l[12:], uint64(off))
		// Syscall, not FcntlInt: converting &l in the call to an assembly
		// function keeps l from moving with the stack while the kernel writes it.
		if _, _, errno := unix.Syscall(unix.SYS_FCNTL, uintptr(fd), unix.F_LOG2PHYS_EXT, //nolint:staticcheck // SA1019: pointer argument must stay pinned
			uintptr(unsafe.Pointer(&l))); errno != 0 {
			return nil, false
		}
		n := int64(binary.LittleEndian.Uint64(l[4:]))
		dev := int64(binary.LittleEndian.Uint64(l[12:]))
		if n <= 0 {
			return nil, false
		}
		// A hole has devoffset -1. Round the EOF extent up to its block.
		if dev >= 0 {
			xs = append(xs, extent{dev, dev + (n+apfsBlock-1)/apfsBlock*apfsBlock})
		}
		off += n
	}
	return xs, true
}

// ATTR_CMNEXT_* from <sys/attr.h>, returned after a u32 length as u64, u64, u32.
const (
	attrCmnExtCloneID     = 0x00000100
	attrCmnExtExtFlags    = 0x00000200
	attrCmnExtCloneRefcnt = 0x00001000
	efMayShareBlocks      = 0x00000001 // EF_MAY_SHARE_BLOCKS, <sys/stat.h>
)

// fileCloneInfo reads a file's APFS clone attributes. ok is false when the
// volume cannot report them; the caller then reads extents.
func fileCloneInfo(path string) (ci cloneInfo, ok bool) {
	p, err := unix.BytePtrFromString(path)
	if err != nil {
		return ci, false
	}
	al := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Forkattr:    attrCmnExtCloneID | attrCmnExtExtFlags | attrCmnExtCloneRefcnt,
	}
	var buf [32]byte
	_, _, errno := unix.Syscall6(unix.SYS_GETATTRLIST, //nolint:staticcheck // SA1019: no libSystem wrapper exists
		uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&al)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		unix.FSOPT_NOFOLLOW|unix.FSOPT_ATTR_CMN_EXTENDED, 0)
	if errno != 0 || binary.LittleEndian.Uint32(buf[0:]) < 24 {
		return ci, false
	}
	ci.id = binary.LittleEndian.Uint64(buf[4:])
	ci.mayShare = binary.LittleEndian.Uint64(buf[12:])&efMayShareBlocks != 0
	ci.refcnt = binary.LittleEndian.Uint32(buf[20:])
	return ci, true
}
