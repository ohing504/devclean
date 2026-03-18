package scanner

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
func (r *Registry) ScanWithProgress(ctx context.Context, root string, scanners []Scanner, onProgress ProgressFunc) ([]model.ScanResult, error) {
	var all []model.ScanResult

	// Attach a real-time progress reporter via context
	scanCtx := ctx
	if onProgress != nil {
		scanCtx = WithProgress(ctx, func(found int) {
			onProgress("", len(all)+found)
		})
	}

	for _, s := range scanners {
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

// DirSize calculates the disk usage of a directory using `du -sk`.
// Falls back to walking the filesystem if du is not available.
func DirSize(path string) int64 {
	cmd := exec.Command("du", "-sk", path)
	out, err := cmd.Output()
	if err == nil {
		parts := strings.SplitN(strings.TrimSpace(string(out)), "\t", 2)
		if kb, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
			return kb * 1024 // KB to bytes
		}
	}

	// Fallback: walk filesystem
	var size int64
	_ = fs.WalkDir(os.DirFS(path), ".", func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

// ModTime returns the modification time of a path, or zero time on error.
// Delegates to pathutil.ModTime.
func ModTime(path string) time.Time {
	return pathutil.ModTime(path)
}
