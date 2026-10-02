package scanner

import (
	"encoding/binary"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// apfsBlock is the APFS allocation block size.
const apfsBlock = 4096

// cloneSizingSupported reports whether fileExtents can read physical extents.
const cloneSizingSupported = true

// fileExtents returns the physical extents of a file's data via
// fcntl(F_LOG2PHYS_EXT), walking from offset 0 to size. ok is false when the
// file cannot be opened or a lookup fails (e.g. a compressed file whose data
// lives outside the data fork); the caller then counts its blocks as-is.
func fileExtents(path string, size int64) (xs []extent, ok bool) {
	if size == 0 {
		return nil, true
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()

	// struct log2phys is #pragma pack(4): u32 l2p_flags, off_t
	// l2p_contigbytes (in: bytes to query, out: contiguous bytes), off_t
	// l2p_devoffset (in: file offset, out: device offset).
	var l [20]byte
	for off := int64(0); off < size; {
		binary.LittleEndian.PutUint32(l[0:], 0)
		binary.LittleEndian.PutUint64(l[4:], uint64(size-off))
		binary.LittleEndian.PutUint64(l[12:], uint64(off))
		if _, err := unix.FcntlInt(f.Fd(), unix.F_LOG2PHYS_EXT, int(uintptr(unsafe.Pointer(&l)))); err != nil {
			return nil, false
		}
		n := int64(binary.LittleEndian.Uint64(l[4:]))
		dev := int64(binary.LittleEndian.Uint64(l[12:]))
		if n <= 0 {
			return nil, false
		}
		// The last extent ends at EOF; round it to the block it occupies.
		xs = append(xs, extent{dev, dev + (n+apfsBlock-1)/apfsBlock*apfsBlock})
		off += n
	}
	return xs, true
}

// getattrlist(2) request for the common extended attributes ATTR_CMNEXT_CLONEID
// (u_int64_t), ATTR_CMNEXT_EXT_FLAGS (u_int64_t) and ATTR_CMNEXT_CLONE_REFCNT
// (u_int32_t), returned in that order after a u_int32_t buffer length.
const (
	attrCmnExtCloneID     = 0x00000100
	attrCmnExtExtFlags    = 0x00000200
	attrCmnExtCloneRefcnt = 0x00001000
	efMayShareBlocks      = 0x00000001 // EF_MAY_SHARE_BLOCKS, <sys/stat.h>
)

// fileCloneInfo reads a file's APFS clone attributes without opening it. ok is
// false when the volume cannot report them.
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
	// Neither syscall nor x/sys exposes a libSystem getattrlist wrapper, so
	// this is a direct syscall. If it ever fails, the caller reads extents
	// instead: slower, same result.
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
