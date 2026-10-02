package scanner

import (
	"cmp"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"

	"github.com/ohing504/devclean/internal/fstree"
	"github.com/ohing504/devclean/internal/model"
)

// Clone-aware sizing (CloneAware results). An APFS clone is a separate inode
// sharing its source's blocks, so st_blocks counts shared blocks once per file.
//   - A pure clone reports a clone ID and how many files share it; its blocks
//     count once, only when all those files are inside the artifact. Otherwise
//     they go to CloneShares for DedupedTotal.
//   - Any other file that may share blocks has its physical extents read;
//     overlaps count once and ranges the clone source uses are left out.
// Blocks held only by an APFS snapshot are not seen.

type cloneInfo struct {
	id       uint64 // equal for pure clones of each other
	refcnt   uint32 // number of those clones, this file included
	mayShare bool
}

type cloneGroup struct {
	model.CloneShare        // Blocks: st_blocks of one member
	path             string // one member, for reading its extents
	size             int64
}

// extent is a physical byte range [start, end) on a device.
type extent struct{ start, end int64 }

// mergeExtents sorts ranges and merges overlapping or adjacent ones in place.
func mergeExtents(xs []extent) []extent {
	if len(xs) == 0 {
		return xs
	}
	slices.SortFunc(xs, func(a, b extent) int { return cmp.Compare(a.start, b.start) })
	out := xs[:1]
	for _, x := range xs[1:] {
		last := &out[len(out)-1]
		if x.start <= last.end {
			last.end = max(last.end, x.end)
			continue
		}
		out = append(out, x)
	}
	return out
}

// bytesNotIn returns the length of the union of xs that lies outside ref. Both
// must be merged (sorted, non-overlapping).
func bytesNotIn(xs, ref []extent) int64 {
	var total int64
	j := 0
	for _, x := range xs {
		total += x.end - x.start
		for j < len(ref) && ref[j].end <= x.start {
			j++
		}
		for k := j; k < len(ref) && ref[k].start < x.end; k++ {
			total -= min(x.end, ref[k].end) - max(x.start, ref[k].start)
		}
	}
	return total
}

// statDev returns the device of path, following a symlink.
func statDev(path string) (uint64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(sys.Dev), true //nolint:unconvert // Dev is int32 on darwin, uint64 on linux
}

// cloneSources loads each clone source's extents once per sizing pass; make
// it with newCloneSources.
type cloneSources struct {
	mu     sync.Mutex
	byPath map[string]*cloneSource
}

func newCloneSources() *cloneSources {
	return &cloneSources{byPath: make(map[string]*cloneSource)}
}

type cloneSource struct {
	once    sync.Once
	dev     uint64
	ok      bool
	extents []extent
}

// get returns the merged extents of the files under path and their device; ok
// is false when path is missing. Files whose extents cannot be read are left
// out, so their shared blocks count as freed.
func (c *cloneSources) get(ctx context.Context, path string) (extents []extent, dev uint64, ok bool) {
	c.mu.Lock()
	s := c.byPath[path]
	if s == nil {
		s = &cloneSource{}
		c.byPath[path] = s
	}
	c.mu.Unlock()

	s.once.Do(func() {
		if s.dev, s.ok = statDev(path); !s.ok {
			return
		}
		var xs []extent
		w := fstree.Walker{
			FollowRoot: true,
			Entries: func(dir string, entries []fs.DirEntry) bool {
				for _, e := range entries {
					if !e.Type().IsRegular() {
						continue
					}
					if info, err := e.Info(); err == nil {
						if fx, ok := fileExtents(filepath.Join(dir, e.Name()), info.Size()); ok {
							xs = append(xs, fx...)
						}
					}
				}
				return true
			},
		}
		if _, err := w.Walk(ctx, path); err != nil {
			s.ok = false
		}
		s.extents = mergeExtents(xs)
	})
	return s.extents, s.dev, s.ok
}
