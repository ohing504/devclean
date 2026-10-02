package scanner

import (
	"cmp"
	"context"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"syscall"

	"github.com/ohing504/devclean/internal/fstree"
)

// Clone-aware sizing. An APFS clone is a separate inode that shares its
// source's physical blocks until either side is rewritten, so st_blocks counts
// shared blocks once per file and neither block counting nor inode dedup sees
// the sharing. For artifacts known to be made of clones (browser code-sign
// copies, pnpm node_modules on macOS), sizing asks APFS about each file:
//
//   - A pure clone (all blocks shared) reports a clone ID and how many files
//     share it. Its blocks count once, and only when every one of those files
//     is inside the artifact; otherwise they go to CloneShares so a total over
//     several artifacts can count them. pnpm-installed files are of this kind.
//   - A file that may share some blocks has its physical extents read: blocks
//     shared inside the artifact count once, and blocks the clone source (the
//     installed app) also uses are not counted. Browser code-sign copies are of
//     this kind. The source is read only when such a file exists.
//
// Extents are compared as device byte ranges. Blocks held only by an APFS
// snapshot are not seen: deleting them frees nothing until the snapshot goes.

// cloneInfo holds a file's APFS clone attributes: id is shared by pure clones
// of each other, refcnt counts those files (itself included), and mayShare is
// set when the file may share some blocks with another file.
type cloneInfo struct {
	id       uint64
	refcnt   uint32
	mayShare bool
}

// cloneGroup tallies the pure clones of one clone ID found in an artifact.
type cloneGroup struct {
	refcnt uint32
	seen   uint32
	blocks int64  // st_blocks of one member
	path   string // one member, read for extents when needed
	size   int64
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

// cloneSources loads each clone source's extents once per sizing pass, shared
// by the sizing workers.
type cloneSources struct {
	mu     sync.Mutex
	byPath map[string]*cloneSource
}

type cloneSource struct {
	once    sync.Once
	dev     uint64
	ok      bool
	extents []extent
}

// get returns the merged extents of every file under path and the device they
// are on; ok is false when path is missing or its extents cannot be read.
func (c *cloneSources) get(ctx context.Context, path string) (extents []extent, dev uint64, ok bool) {
	c.mu.Lock()
	if c.byPath == nil {
		c.byPath = make(map[string]*cloneSource)
	}
	s := c.byPath[path]
	if s == nil {
		s = &cloneSource{}
		c.byPath[path] = s
	}
	c.mu.Unlock()

	s.once.Do(func() { s.load(ctx, path) })
	return s.extents, s.dev, s.ok
}

// load reads the extents of every regular file under path. The walk only
// lists files; their extents are read by a worker pool, since one lookup per
// file is syscall-bound and a pnpm store holds hundreds of thousands of files.
func (s *cloneSource) load(ctx context.Context, path string) {
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return
	}
	files := make(chan fileRef, 1024)
	results := make(chan []extent, 1024)
	var wg sync.WaitGroup
	for range min(runtime.NumCPU(), sizeWorkerCap) {
		wg.Go(func() {
			for f := range files {
				if xs, ok := fileExtents(f.path, f.size); ok {
					results <- xs
				}
			}
		})
	}
	var xs []extent
	collected := make(chan struct{})
	go func() {
		for r := range results {
			xs = append(xs, r...)
		}
		close(collected)
	}()

	w := fstree.Walker{
		Enter: func(dir string, info fs.FileInfo) bool {
			if dir == root {
				if st, ok := info.Sys().(*syscall.Stat_t); ok {
					s.dev = uint64(st.Dev)
					s.ok = true
				}
			}
			return true
		},
		Entries: func(dir string, entries []fs.DirEntry) bool {
			for _, e := range entries {
				if !e.Type().IsRegular() {
					continue
				}
				if info, err := e.Info(); err == nil {
					files <- fileRef{filepath.Join(dir, e.Name()), info.Size()}
				}
			}
			return true
		},
	}
	_, err = w.Walk(ctx, root)
	close(files)
	wg.Wait()
	close(results)
	<-collected
	if err != nil {
		s.ok = false
		return
	}
	// Files whose extents could not be read (unreadable, compressed) are
	// missing from the source, so blocks a copy shares with them count as
	// freed: the size errs toward the per-file block count, never below it.
	s.extents = mergeExtents(xs)
}

type fileRef struct {
	path string
	size int64
}
