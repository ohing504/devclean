// Package fstree is the one tree traversal shared by finding artifacts,
// sizing them and checking them before deletion, so all three cover the same
// files. It never follows symlinks (only the root, when asked) and stays on
// the root's filesystem, like du -x. It never reads a dataless directory, whose
// content is in a cloud provider and would be downloaded by the read.
package fstree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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

// dataless is indirected so a test can mark a directory dataless without a
// cloud provider.
var dataless = isDataless

// Walker walks a tree depth-first, reading each directory once. S is what a
// directory hands its subdirectories (struct{} for nothing); the root gets the
// zero S. Every hook is optional.
type Walker[S any] struct {
	// FollowRoot reads through a symlinked root, for a user-given location
	// such as --path. Leave it off when the root is the item to delete:
	// deleting a symlink removes only the link.
	FollowRoot bool
	// Slots, when set, bounds extra goroutines: a subdirectory is walked in
	// its own goroutine while a slot is free. Hooks then run concurrently.
	Slots chan struct{}
	// Enter is called before a directory is read (and for a non-directory
	// root, which is not read), with its parent's S. Returning false skips it.
	Enter func(dir string, info fs.FileInfo, parent S) bool
	// Entries is called once a directory is read, with its parent's S, and
	// returns the S for its subdirectories. Returning false skips them. With
	// Slots the subdirectories run concurrently on the same S, so neither hook
	// may modify a parent S in place.
	Entries func(dir string, entries []fs.DirEntry, parent S) (S, bool)
}

// Walk walks the tree under root and returns the directories it could not
// check, sorted: on another filesystem, of unknown filesystem, dataless, or
// unreadable. A root that does not exist is an empty tree. The only error is
// ctx's.
func (w Walker[S]) Walk(ctx context.Context, root string) ([]string, error) {
	statRoot := os.Lstat
	if w.FollowRoot {
		statRoot = os.Stat
	}
	info, err := statRoot(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil || (info.IsDir() && dataless(info)) {
		return []string{root}, nil
	}
	rootDev, rootDevOK := deviceOf(root, info)

	var mu sync.Mutex // guards skipped
	var skipped []string
	skip := func(dir string) {
		mu.Lock()
		skipped = append(skipped, dir)
		mu.Unlock()
	}
	var wg sync.WaitGroup
	var visit func(dir string, info fs.FileInfo, parent S)
	visit = func(dir string, info fs.FileInfo, parent S) {
		if ctx.Err() != nil {
			return
		}
		if w.Enter != nil && !w.Enter(dir, info, parent) {
			return
		}
		if !info.IsDir() {
			return
		}
		entries, err := readDir(dir)
		if err != nil {
			skip(dir)
			return
		}
		state := parent
		if w.Entries != nil {
			var ok bool
			if state, ok = w.Entries(dir, entries, parent); !ok {
				return
			}
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
			if err != nil || dataless(childInfo) {
				skip(child)
				continue
			}
			if rootDevOK {
				if dev, ok := deviceOf(child, childInfo); !ok || dev != rootDev {
					skip(child)
					continue
				}
			}
			select {
			case w.Slots <- struct{}{}: // never ready when Slots is nil
				wg.Go(func() {
					defer func() { <-w.Slots }()
					visit(child, childInfo, state)
				})
			default:
				visit(child, childInfo, state)
			}
		}
	}
	var zero S
	visit(root, info, zero)
	wg.Wait()
	slices.Sort(skipped)
	return skipped, ctx.Err()
}

// UncheckedError reports a tree with directories Walk could not check.
type UncheckedError struct {
	Path string
	Dirs []string
}

func (e *UncheckedError) Error() string {
	return fmt.Sprintf("refusing to delete %s: contains directories on another filesystem, not downloaded from a cloud provider, or unreadable (%s)",
		e.Path, strings.Join(e.Dirs, ", "))
}

// CheckOneFilesystem returns an *UncheckedError when the tree at path has
// directories Walk could not check. A recursive delete does not stop at
// them, so it would remove files that were never sized or shown.
func CheckOneFilesystem(ctx context.Context, path string) error {
	skipped, err := Walker[struct{}]{}.Walk(ctx, path)
	if err != nil {
		return err
	}
	if len(skipped) > 0 {
		return &UncheckedError{Path: path, Dirs: skipped}
	}
	return nil
}
