package cleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ohing504/devclean/internal/model"
)

// Options controls cleaner behavior.
type Options struct {
	Force    bool   // permanent delete (skip Trash)
	DryRun   bool   // preview only
	TrashDir string // custom trash dir (for testing; empty = system default)
}

// Cleaner handles deletion of scan results.
type Cleaner struct {
	opts Options
}

// New creates a cleaner with the given options.
func New(opts Options) *Cleaner {
	return &Cleaner{opts: opts}
}

// Clean deletes or trashes a scan result item.
func (c *Cleaner) Clean(r model.ScanResult) error {
	if r.Protected {
		return fmt.Errorf("refusing to delete protected item: %s (%s)", r.Path, r.Reason)
	}

	if c.opts.DryRun {
		return nil
	}

	if c.opts.Force {
		return os.RemoveAll(r.Path)
	}

	return c.moveToTrash(r.Path)
}

// CleanResult holds the outcome of a single clean operation.
type CleanResult struct {
	Item  model.ScanResult
	Error error
}

// CleanAll cleans multiple results and returns per-item outcomes.
func (c *Cleaner) CleanAll(results []model.ScanResult) []CleanResult {
	var out []CleanResult
	for _, r := range results {
		err := c.Clean(r)
		out = append(out, CleanResult{Item: r, Error: err})
	}
	return out
}

func (c *Cleaner) moveToTrash(path string) error {
	trashDir := c.opts.TrashDir
	if trashDir == "" {
		trashDir = defaultTrashDir()
	}

	if err := os.MkdirAll(trashDir, 0o755); err != nil {
		return fmt.Errorf("cannot create trash dir: %w", err)
	}

	return moveToDir(path, trashDir)
}

func defaultTrashDir() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".Trash")
	}
	// Linux: XDG trash
	xdgData := os.Getenv("XDG_DATA_HOME")
	if xdgData == "" {
		xdgData = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdgData, "Trash", "files")
}

func moveToDir(src, destDir string) error {
	name := filepath.Base(src)
	dest := filepath.Join(destDir, name)

	// Handle name conflicts
	if _, err := os.Stat(dest); err == nil {
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("%s_%d", dest, i)
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				dest = candidate
				break
			}
		}
	}

	return os.Rename(src, dest)
}
