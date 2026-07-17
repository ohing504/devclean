package cleaner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

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

// Clean reclaims a scan result item: protection and dry-run gates apply to
// every strategy; execution then goes through the item's DeleteMethod when one
// is attached, or falls back to path removal (trash or permanent).
func (c *Cleaner) Clean(ctx context.Context, r model.ScanResult) error {
	if r.Protected {
		return fmt.Errorf("refusing to delete protected item: %s (%s)", r.Path, r.Reason)
	}

	if c.opts.DryRun {
		return nil
	}

	if r.Delete != nil {
		// A method without Run is a scanner bug; refuse rather than fall back
		// to path removal, which could delete a path the method never meant to.
		if r.Delete.Run == nil {
			return fmt.Errorf("delete method for %s has no Run function", r.Path)
		}
		return r.Delete.Run(ctx)
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
func (c *Cleaner) CleanAll(ctx context.Context, results []model.ScanResult) []CleanResult {
	var out []CleanResult
	for _, r := range results {
		err := c.Clean(ctx, r)
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

	err := os.Rename(src, dest)
	if err == nil {
		return nil
	}
	if !shouldFallBackToCopy(err) {
		return err
	}
	return moveByCopy(src, dest)
}

// shouldFallBackToCopy reports whether a failed os.Rename should retry via
// copy-then-remove. os.Rename fails with EXDEV when src and dest are on
// different filesystems (external drive, separate partition); the Trash lives
// on the home volume, so an artifact on any other volume always hits this. Any
// other error is a genuine failure and must propagate untouched, not be masked
// behind a copy attempt.
func shouldFallBackToCopy(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// moveByCopy is the cross-device fallback for moveToDir: copy the tree to dest,
// then remove the original. On copy failure the original is left intact and the
// partial copy is removed so a retry starts clean; deletion happens only after
// a fully successful copy.
func moveByCopy(src, dest string) error {
	if err := copyTree(src, dest); err != nil {
		_ = os.RemoveAll(dest)
		return fmt.Errorf("cross-device move of %s: %w", src, err)
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("copied %s to trash but could not remove original: %w", src, err)
	}
	return nil
}

// copyTree recursively copies src to dest, recreating directories, regular
// files (contents + permission bits), and symlinks (as links, not their
// targets). Symlinks are recreated rather than followed so a link into a huge
// or cyclic target is never walked.
func copyTree(src, dest string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dest)
	case info.IsDir():
		perm := info.Mode().Perm()
		// Create it owner-writable so children copy in even if perm is read-only,
		// then restore the exact mode after the subtree is populated. os.MkdirAll
		// ANDs the mode with the umask, so the final Chmod is what preserves the
		// exact bits (a group/other-writable dir would otherwise lose them).
		if err := os.MkdirAll(dest, perm|0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name())); err != nil {
				return err
			}
		}
		return os.Chmod(dest, perm)
	case info.Mode().IsRegular():
		return copyFile(src, dest, info.Mode().Perm())
	default:
		return fmt.Errorf("unsupported file type: %s", src)
	}
}

func copyFile(src, dest string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// os.OpenFile ANDs perm with the umask; Chmod forces the exact source bits
	// so a group/other-writable file keeps them across the copy.
	return os.Chmod(dest, perm)
}
