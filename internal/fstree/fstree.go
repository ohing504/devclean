// Package fstree walks a directory tree under the one traversal policy that
// every stage of devclean shares — finding artifacts, sizing them, and
// checking them before deletion — so what is reported, what is counted and
// what is removed cover the same files:
//
//   - Symlinks are never followed: a symlinked directory is neither descended
//     into nor reported as a directory. Only the root may be read through, and
//     only when the caller asks (Walker.FollowRoot).
//   - The walk stays on the root's filesystem, like du -x. A directory whose
//     device ID differs from the root's — a mount such as Xcode CoreDevice's
//     devicefs (paired iPhones' app containers, measured at ~4 s per directory
//     read), a network share or an external volume — is not entered, nor is
//     one whose device cannot be determined. Both are returned to the caller
//     as skipped instead.
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

// readDir is os.ReadDir, indirected so a test can assert every directory is
// read exactly once — the single-read property is what keeps the walk cheap
// on large trees, and a reintroduced double-read is otherwise invisible.
var readDir = os.ReadDir

// deviceOf returns the device ID of the filesystem info was taken from; ok is
// false when the platform exposes none. Indirected (with the path) so a test
// can place a directory on a "different filesystem" without mounting one.
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
	// FollowRoot reads through a symlinked root (stat instead of lstat). Set
	// it when the root is a user-given location such as --path; leave it off
	// when the root is the item itself, since deleting a symlink removes only
	// the link.
	FollowRoot bool
	// Enter is called for the root and every directory on the root's
	// filesystem, before it is read; info is its lstat (stat for a followed
	// root). The root may also be a file or a symlink — it is passed to Enter
	// and not read. Returning false skips the directory: it is not read and
	// Leave is not called for it.
	Enter func(dir string, info fs.FileInfo) bool
	// Entries is called with the directory's entries once read. Returning
	// false skips its subdirectories; Leave is not called. A directory that
	// cannot be read is skipped without calling Entries.
	Entries func(dir string, entries []fs.DirEntry) bool
	// Leave is called after all subdirectories of a read directory are walked.
	Leave func(dir string)
}

// Walk walks the tree under root and returns the directories it did not enter
// because they sit on another filesystem or their device is unknown. A root
// that does not exist is an empty tree. The only error is ctx's.
func (w Walker) Walk(ctx context.Context, root string) ([]string, error) {
	statRoot := os.Lstat
	if w.FollowRoot {
		statRoot = os.Stat
	}
	info, err := statRoot(root)
	if err != nil {
		return nil, nil
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
			return nil
		}
		if w.Entries != nil && !w.Entries(dir, entries) {
			return nil
		}
		for _, e := range entries {
			// e.IsDir() is false for a symlink (ReadDir uses lstat semantics),
			// which is the no-follow policy.
			if !e.IsDir() {
				continue
			}
			child := filepath.Join(dir, e.Name())
			childInfo, err := e.Info()
			if errors.Is(err, fs.ErrNotExist) {
				continue // removed since the read
			}
			if err != nil {
				skipped = append(skipped, child) // cannot be checked
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
	if err := visit(root, info); err != nil {
		return skipped, err
	}
	return skipped, nil
}

// SpansFilesystemsError reports a tree that reaches onto another filesystem,
// or holds directories whose filesystem cannot be determined.
type SpansFilesystemsError struct {
	Path string
	Dirs []string
}

func (e *SpansFilesystemsError) Error() string {
	return fmt.Sprintf("refusing to delete %s: it contains directories on another filesystem or of unknown filesystem (%s)",
		e.Path, strings.Join(e.Dirs, ", "))
}

// CheckOneFilesystem returns a *SpansFilesystemsError when the tree at path
// (not following a symlinked path) has directories it would not enter. A
// recursive delete does not stop at mounts, so deleting such a tree would
// remove files on the other filesystem that were never sized or shown.
func CheckOneFilesystem(ctx context.Context, path string) error {
	skipped, err := Walker{}.Walk(ctx, path)
	if err != nil {
		return err
	}
	if len(skipped) > 0 {
		return &SpansFilesystemsError{Path: path, Dirs: skipped}
	}
	return nil
}
