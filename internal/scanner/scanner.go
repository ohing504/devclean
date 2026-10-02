package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ohing504/devclean/internal/fstree"
	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
)

// Scanner is the interface every ecosystem scanner implements. Scan receives
// an absolute root: Registry.ScanWithProgress resolves a relative one before
// any scanner runs.
type Scanner interface {
	Name() string
	Ecosystem() model.Ecosystem
	Scan(ctx context.Context, root string) ([]model.ScanResult, error)
}

// VendorCleanup describes an ecosystem-native cleanup action that delegates to
// an official tool (e.g. `xcrun simctl delete unavailable` for Xcode). These
// run alongside path-based cleanup but use the vendor's own command so internal
// state stays consistent. It is the ecosystem-level bulk counterpart of a
// per-item ScanResult.Delete: both share the model.DeleteMethod execution
// contract (Kind, Display for dry-run, Run to execute).
type VendorCleanup struct {
	ID          string // stable identifier, e.g. "simctl-delete-unavailable"
	Description string // user-facing summary
	model.DeleteMethod
}

// VendorCleaner is implemented by scanners that contribute vendor-native
// cleanup actions. Scanners without vendor commands simply do not implement it.
type VendorCleaner interface {
	VendorCleanups() []VendorCleanup
}

// Registry holds all registered scanners and orchestrates scanning.
type Registry struct {
	scanners []Scanner
}

// NewRegistry creates an empty scanner registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a scanner to the registry.
func (r *Registry) Register(s Scanner) {
	r.scanners = append(r.scanners, s)
}

// All returns all registered scanners.
func (r *Registry) All() []Scanner {
	return r.scanners
}

// ForEcosystems filters scanners by ecosystem list.
func (r *Registry) ForEcosystems(ecos []model.Ecosystem) []Scanner {
	set := make(map[model.Ecosystem]bool, len(ecos))
	for _, e := range ecos {
		set[e] = true
	}

	var filtered []Scanner
	for _, s := range r.scanners {
		if set[s.Ecosystem()] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// ScanAll runs all registered scanners sequentially and collects results.
func (r *Registry) ScanAll(ctx context.Context, root string) ([]model.ScanResult, error) {
	return r.ScanWith(ctx, root, r.scanners)
}

// ProgressFunc is called during scanning with the current ecosystem name and total items found so far.
type ProgressFunc func(ecosystem string, totalFound int)

type progressKey struct{}

// WithProgress attaches a progress callback to a context.
// Scanners call ReportProgress to notify the caller of new items found.
func WithProgress(ctx context.Context, fn func(int)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

// ReportProgress calls the progress callback if one is attached to the context.
func ReportProgress(ctx context.Context, count int) {
	if fn, ok := ctx.Value(progressKey{}).(func(int)); ok {
		fn(count)
	}
}

// ScanWith runs a subset of scanners and collects results.
func (r *Registry) ScanWith(ctx context.Context, root string, scanners []Scanner) ([]model.ScanResult, error) {
	return r.ScanWithProgress(ctx, root, scanners, nil)
}

// ScanWithProgress runs scanners with an optional progress callback.
// Walk-based scanners are partitioned out and executed first as a single
// batched filesystem pass; the remaining scanners run sequentially.
func (r *Registry) ScanWithProgress(ctx context.Context, root string, scanners []Scanner, onProgress ProgressFunc) ([]model.ScanResult, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var all []model.ScanResult

	// Attach a real-time progress reporter via context
	scanCtx := ctx
	if onProgress != nil {
		scanCtx = WithProgress(ctx, func(found int) {
			onProgress("", len(all)+found)
		})
	}

	walkTables, rest := partitionScanners(scanners)

	if len(walkTables) > 0 {
		if onProgress != nil {
			onProgress("projects", len(all))
		}
		results, err := runWalk(scanCtx, root, walkTables)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}

	for _, s := range rest {
		if onProgress != nil {
			onProgress(s.Name(), len(all))
		}
		results, err := s.Scan(scanCtx, root)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}
	return all, nil
}

// partitionScanners splits scanners into walk-engine tables (batched into a
// single filesystem pass) and the remaining scanners (run sequentially).
// Table order follows the scanners' order, which mirrors the registry order.
func partitionScanners(scanners []Scanner) ([]walkEcosystem, []Scanner) {
	var tables []walkEcosystem
	var rest []Scanner
	for _, s := range scanners {
		if w, ok := s.(*walkScanner); ok {
			tables = append(tables, w.table)
		} else {
			rest = append(rest, s)
		}
	}
	return tables, rest
}

// SizeStat holds the apparent (sum of logical file sizes) and disk (allocated
// blocks) bytes for a path, plus the hard-linked inodes it counted so a caller
// can dedup blocks shared across artifacts (e.g. pnpm store ↔ node_modules).
type SizeStat struct {
	Apparent int64
	Disk     int64
	Links    map[model.InodeKey]int64 // Nlink>1 inode → disk blocks, keyed by (dev, ino)
	// Reclaim is what deleting path frees. It equals Disk unless clone-aware
	// sizing ran: then blocks shared between the artifact's files count once
	// and blocks also used by the clone source are left out.
	Reclaim int64
	// CloneShares are the pure clone groups only partly inside path.
	CloneShares map[model.CloneKey]model.CloneShare
}

// cloneSizing turns on clone-aware sizing for one measure call. source, when
// set, returns the clone source's merged extents and their device; it is only
// called when some file is not a pure clone and its extents had to be read.
type cloneSizing struct {
	source func() (extents []extent, dev uint64, ok bool)
}

// Measure walks path with fstree, the traversal deletion is checked with, and
// returns its apparent and disk sizes.
//
// Disk uses st_blocks×512 (allocated blocks), so it stays correct for sparse
// files where the logical size vastly exceeds what is on disk, and matches
// `du`. Apparent sums logical file sizes. Directories contribute their own
// blocks to Disk (ext4 dirs use real blocks; APFS reports ~0) but not to
// Apparent. Files hard-linked more than once are counted once within this
// artifact and recorded in Links so a caller can net out blocks shared across
// artifacts.
func Measure(path string) SizeStat {
	return measure(context.Background(), path, nil)
}

// measure is Measure that stops once ctx is done. A non-nil clones also reads
// each singly-linked file's APFS clone attributes, and physical extents where
// those do not settle it, to fill Reclaim (see clone.go).
func measure(ctx context.Context, path string, clones *cloneSizing) SizeStat {
	var st SizeStat
	seen := make(map[model.InodeKey]struct{})
	var extents []extent
	var extentBlocks int64 // st_blocks of the files whose extents were read
	groups := make(map[uint64]*cloneGroup)
	var groupBlocks int64 // st_blocks of the pure clones grouped by clone ID
	var rootDev uint64
	// The file is filepath.Join(dir, name); it is only built when its extents
	// are read, so plain sizing allocates no paths.
	add := func(dir, name string, info fs.FileInfo) {
		sys, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			if !info.IsDir() && info.Mode()&fs.ModeSymlink == 0 {
				st.Apparent += info.Size() // non-unix fallback: apparent only
			}
			return
		}
		blocks := int64(sys.Blocks) * 512
		if info.IsDir() {
			st.Disk += blocks
			return
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return
		}
		if sys.Nlink > 1 {
			// Count a multiply-linked inode once per artifact for both apparent
			// and disk (matches `du`/`du -A` within-call dedup), and record it
			// so DedupedTotal can net it out across artifacts.
			key := model.InodeKey{Dev: uint64(sys.Dev), Ino: uint64(sys.Ino)}
			if _, dup := seen[key]; dup {
				return
			}
			seen[key] = struct{}{}
			if st.Links == nil {
				st.Links = make(map[model.InodeKey]int64)
			}
			st.Links[key] = blocks
		} else if clones != nil {
			full := filepath.Join(dir, name)
			ci, ok := fileCloneInfo(full)
			switch {
			case ok && ci.refcnt > 1:
				// A pure clone: refcnt files share all of these blocks.
				g := groups[ci.id]
				if g == nil {
					g = &cloneGroup{refcnt: ci.refcnt, blocks: blocks, path: full, size: info.Size()}
					groups[ci.id] = g
				}
				g.seen++
				groupBlocks += blocks
			case !ok || ci.mayShare:
				if xs, ok := fileExtents(full, info.Size()); ok {
					extents = append(extents, xs...)
					extentBlocks += blocks
				}
			}
		}
		st.Apparent += info.Size()
		st.Disk += blocks
	}
	w := fstree.Walker{
		Enter: func(dir string, info fs.FileInfo) bool {
			if dir == path {
				if sys, ok := info.Sys().(*syscall.Stat_t); ok {
					rootDev = uint64(sys.Dev)
				}
			}
			add(dir, "", info)
			return true
		},
		Entries: func(dir string, entries []fs.DirEntry) bool {
			for _, e := range entries {
				if e.IsDir() {
					continue // counted on Enter, if on this filesystem
				}
				if info, err := e.Info(); err == nil {
					add(dir, e.Name(), info)
				}
			}
			return true
		},
	}
	_, _ = w.Walk(ctx, path)
	st.Reclaim = st.Disk
	if clones != nil {
		st.Reclaim = st.Disk - groupBlocks - extentBlocks
		for id, g := range groups {
			switch {
			case g.seen < g.refcnt:
				// Clones outside path keep the blocks; a total over several
				// artifacts may still hold them all.
				if st.CloneShares == nil {
					st.CloneShares = make(map[model.CloneKey]model.CloneShare)
				}
				st.CloneShares[model.CloneKey{Dev: rootDev, ID: id}] = model.CloneShare{Refcnt: g.refcnt, Seen: g.seen, Blocks: g.blocks}
			case len(extents) > 0:
				// A partly rewritten clone in path may share these blocks:
				// count them through the extent union.
				if xs, ok := fileExtents(g.path, g.size); ok {
					extents = append(extents, xs...)
				} else {
					st.Reclaim += g.blocks
				}
			default:
				st.Reclaim += g.blocks
			}
		}
		if len(extents) > 0 {
			var src []extent
			if clones.source != nil {
				if xs, dev, ok := clones.source(); ok && dev == rootDev {
					src = xs
				}
			}
			st.Reclaim += bytesNotIn(mergeExtents(extents), src)
		}
	}
	return st
}

// DirSize returns the disk usage (allocated blocks) of a path. Thin wrapper over
// Measure for callers that only need the disk figure.
func DirSize(path string) int64 {
	return Measure(path).Disk
}

// ModTime returns the modification time of a path, or zero time on error.
// Delegates to pathutil.ModTime.
func ModTime(path string) time.Time {
	return pathutil.ModTime(path)
}
