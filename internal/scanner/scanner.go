package scanner

import (
	"context"
	"io/fs"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
)

// Scanner is the interface every ecosystem scanner implements.
type Scanner interface {
	Name() string
	Ecosystem() model.Ecosystem
	Scan(ctx context.Context, root string) ([]model.ScanResult, error)
}

// VendorCleanup describes an ecosystem-native cleanup action that delegates to
// an official tool (e.g. `xcrun simctl delete unavailable` for Xcode). These
// run alongside path-based cleanup but use the vendor's own command so internal
// state stays consistent.
type VendorCleanup struct {
	ID          string                          // stable identifier, e.g. "simctl-delete-unavailable"
	Description string                          // user-facing summary
	Command     string                          // command that would be run, for dry-run / display
	Run         func(ctx context.Context) error // executes the cleanup
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
}

// Measure walks path in-process and returns its apparent and disk sizes.
//
// Disk uses st_blocks×512 (allocated blocks), so it stays correct for sparse
// files where the logical size vastly exceeds what is on disk, and matches
// `du`. Apparent sums logical file sizes. Directories contribute their own
// blocks to Disk (ext4 dirs use real blocks; APFS reports ~0) but not to
// Apparent. Symlinks are not followed — neither the target nor the link's own
// blocks are counted (consistent with the walk engine's no-follow policy).
// Files hard-linked more than once are counted once within this artifact and
// recorded in Links so a caller can net out blocks shared across artifacts.
func Measure(path string) SizeStat {
	var st SizeStat
	seen := make(map[model.InodeKey]struct{})
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable / racing entries, keep summing the rest
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		sys, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			if !d.IsDir() && d.Type()&fs.ModeSymlink == 0 {
				st.Apparent += info.Size() // non-unix fallback: apparent only
			}
			return nil
		}
		blocks := int64(sys.Blocks) * 512
		if d.IsDir() {
			st.Disk += blocks
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if sys.Nlink > 1 {
			// Count a multiply-linked inode once per artifact for both apparent
			// and disk (matches `du`/`du -A` within-call dedup), and record it
			// so DedupedTotal can net it out across artifacts.
			key := model.InodeKey{Dev: uint64(sys.Dev), Ino: uint64(sys.Ino)}
			if _, dup := seen[key]; dup {
				return nil
			}
			seen[key] = struct{}{}
			if st.Links == nil {
				st.Links = make(map[model.InodeKey]int64)
			}
			st.Links[key] = blocks
		}
		st.Apparent += info.Size()
		st.Disk += blocks
		return nil
	})
	return st
}

// DirSize returns the disk usage (allocated blocks) of a path. Thin wrapper over
// Measure for callers that only need the disk figure.
func DirSize(path string) int64 {
	return Measure(path).Disk
}

// sized fills Size, ApparentSize and Links on r by measuring r.Path — the
// single-result form of what the walk engine's sizePending does in bulk (the
// stat-based scanners now defer to sizePending; this remains for callers that
// size one result at a time).
func sized(r model.ScanResult) model.ScanResult {
	st := Measure(r.Path)
	r.Size = st.Disk
	r.ApparentSize = st.Apparent
	r.Links = st.Links
	return r
}

// ModTime returns the modification time of a path, or zero time on error.
// Delegates to pathutil.ModTime.
func ModTime(path string) time.Time {
	return pathutil.ModTime(path)
}
