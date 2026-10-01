// Package fstree is the one tree traversal shared by finding artifacts,
// sizing them and checking them before deletion, so all three cover the same
// files. It never follows symlinks (only the root, when asked) and stays on
// the root's filesystem, like du -x.
package fstree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// readDir is indirected so a test can assert each directory is read once.
var readDir = os.ReadDir

// deviceOf is indirected so a test can place a directory on another
// filesystem without mounting one.
var deviceOf = func(_ string, info fs.FileInfo) (uint64, bool) {
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(sys.Dev), true //nolint:unconvert // Dev is int32 on darwin, uint64 on linux
}

// Walker walks a tree depth-first, reading each directory once. Every hook is
// optional.
type Walker struct {
	// FollowRoot reads through a symlinked root, for a user-given location
	// such as --path. Leave it off when the root is the item to delete:
	// deleting a symlink removes only the link.
	FollowRoot bool
	// Enter is called before a directory is read (and for a non-directory
	// root, which is not read). Returning false skips it.
	Enter func(dir string, info fs.FileInfo) bool
	// Entries is called once a directory is read. Returning false skips its
	// subdirectories and Leave.
	Entries func(dir string, entries []fs.DirEntry) bool
	// Leave is called after a read directory's subtree is walked.
	Leave func(dir string)
}

// Walk walks the tree under root and returns the directories it could not
// check: on another filesystem, of unknown filesystem, or unreadable. A root
// that does not exist is an empty tree. The only error is ctx's.
func (w Walker) Walk(ctx context.Context, root string) ([]string, error) {
	statRoot := os.Lstat
	if w.FollowRoot {
		statRoot = os.Stat
	}
	info, err := statRoot(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return []string{root}, nil
	}
	rootDev, rootDevOK := deviceOf(root, info)

	var skipped []string
	var visit func(dir string, info fs.FileInfo) error
	visit = func(dir string, info fs.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if w.Enter != nil && !w.Enter(dir, info) {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		entries, err := readDir(dir)
		if err != nil {
			skipped = append(skipped, dir)
			return nil
		}
		if w.Entries != nil && !w.Entries(dir, entries) {
			return nil
		}
		for _, e := range entries {
			if !e.IsDir() { // false for symlinks: ReadDir does not follow them
				continue
			}
			child := filepath.Join(dir, e.Name())
			childInfo, err := e.Info()
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				skipped = append(skipped, child)
				continue
			}
			if rootDevOK {
				if dev, ok := deviceOf(child, childInfo); !ok || dev != rootDev {
					skipped = append(skipped, child)
					continue
				}
			}
			if err := visit(child, childInfo); err != nil {
				return err
			}
		}
		if w.Leave != nil {
			w.Leave(dir)
		}
		return nil
	}
	return skipped, visit(root, info)
}

// UncheckedError reports a tree with directories Walk could not check.
type UncheckedError struct {
	Path string
	Dirs []string
}

func (e *UncheckedError) Error() string {
	return fmt.Sprintf("refusing to delete %s: contains directories on another filesystem or unreadable (%s)",
		e.Path, strings.Join(e.Dirs, ", "))
}

// CheckOneFilesystem returns an *UncheckedError when the tree at path has
// directories Walk could not check. A recursive delete does not stop at
// mounts, so it would remove files that were never sized or shown.
func CheckOneFilesystem(ctx context.Context, path string) error {
	skipped, err := Walker{}.Walk(ctx, path)
	if err != nil {
		return err
	}
	if len(skipped) > 0 {
		return &UncheckedError{Path: path, Dirs: skipped}
	}
	return nil
}
