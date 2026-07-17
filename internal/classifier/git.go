package classifier

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/pathutil"
)

// HasUncommittedChanges returns true if the directory is a git repo with uncommitted changes.
func HasUncommittedChanges(dir string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// FindGitRoot returns the git root for a path, or empty string if not in a repo.
func FindGitRoot(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// LastGitCommitTime returns the timestamp of the last git commit in a repo.
func LastGitCommitTime(dir string) time.Time {
	cmd := exec.Command("git", "log", "-1", "--format=%ct")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}
	}
	unix, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

// findProjectRoot walks up from dir looking for the topmost directory
// that contains package.json (or similar project markers).
// This handles non-git projects and broken git worktrees.
func findProjectRoot(dir string) string {
	best := ""
	current := dir
	home, _ := os.UserHomeDir()

	for current != "/" && current != "." && current != home {

		if _, err := os.Stat(filepath.Join(current, "package.json")); err == nil {
			best = current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return best
}

// batchCheckIgnored runs git check-ignore once for all paths under a given git root.
// Returns a set of paths that are gitignored.
func batchCheckIgnored(gitRoot string, paths []string) map[string]bool {
	args := append([]string{"check-ignore"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = gitRoot
	out, _ := cmd.Output()
	ignored := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			ignored[line] = true
		}
	}
	return ignored
}

// gitInfo caches dirty status, last commit time, and dir mtime per git root.
type gitInfo struct {
	dirty      bool
	lastCommit time.Time
	dirMtime   time.Time
}

// gitWorkerCap bounds concurrent git forks so a scan with many repos does not
// spawn a subprocess per repo all at once.
const gitWorkerCap = 8

// distinct returns the input's unique values in first-seen order.
func distinct(vals []string) []string {
	seen := make(map[string]bool, len(vals))
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// parallelMap applies fn to each key across a bounded worker pool and returns
// the key→result map. keys must be distinct (one map entry per key); each
// worker writes a distinct slice index, so no locking is needed. This is the
// classification hot path: each fn forks git (rev-parse / status / log), and
// running them serially over dozens of repos dominated warm-cache scan time.
func parallelMap[T any](keys []string, fn func(string) T) map[string]T {
	out := make(map[string]T, len(keys))
	if len(keys) == 0 {
		return out
	}
	workers := runtime.NumCPU()
	if workers > gitWorkerCap {
		workers = gitWorkerCap
	}
	if workers > len(keys) {
		workers = len(keys)
	}

	vals := make([]T, len(keys))
	idx := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				vals[i] = fn(keys[i])
			}
		}()
	}
	for i := range keys {
		idx <- i
	}
	close(idx)
	wg.Wait()

	for i, k := range keys {
		out[k] = vals[i]
	}
	return out
}

// ApplyGitInfo checks git status and last commit time for each scan result.
// It marks dirty repos as protected and updates LastMod to the more recent of
// filesystem mtime vs git commit time (matching cluttered's behavior).
func ApplyGitInfo(results []model.ScanResult) {
	// First pass: resolve the git root of each result's project dir. FindGitRoot
	// forks `git rev-parse`, so resolve each distinct dir once, concurrently. The
	// findProjectRoot / projectDir fallbacks are pure functions of the dir, so
	// folding them in here is equivalent to the previous per-result application.
	projectDirs := distinct(func() []string {
		ds := make([]string, len(results))
		for i := range results {
			ds[i] = filepath.Dir(results[i].Path)
		}
		return ds
	}())
	rootByDir := parallelMap(projectDirs, func(dir string) string {
		gitRoot := FindGitRoot(dir)
		if gitRoot == "" {
			gitRoot = findProjectRoot(dir)
		}
		if gitRoot == "" {
			gitRoot = dir
		}
		return gitRoot
	})

	roots := make([]string, len(results))
	for i := range results {
		roots[i] = rootByDir[filepath.Dir(results[i].Path)]
	}

	// Second pass: gather git info per distinct root (status + log forks),
	// concurrently across roots.
	cache := parallelMap(distinct(roots), func(gitRoot string) *gitInfo {
		return &gitInfo{
			dirty:      HasUncommittedChanges(gitRoot),
			lastCommit: LastGitCommitTime(gitRoot),
			dirMtime:   pathutil.ModTime(gitRoot),
		}
	})

	// Third pass: collect artifact paths per dirty git root for batch check-ignore
	dirtyRootPaths := make(map[string][]string)
	for i := range results {
		gitRoot := roots[i]
		info := cache[gitRoot]
		if info.dirty {
			dirtyRootPaths[gitRoot] = append(dirtyRootPaths[gitRoot], results[i].Path)
		}
	}

	// Batch check-ignore per dirty root, concurrently across roots.
	dirtyRoots := make([]string, 0, len(dirtyRootPaths))
	for gitRoot := range dirtyRootPaths {
		dirtyRoots = append(dirtyRoots, gitRoot)
	}
	ignoredByRoot := parallelMap(dirtyRoots, func(gitRoot string) map[string]bool {
		return batchCheckIgnored(gitRoot, dirtyRootPaths[gitRoot])
	})

	// Fourth pass: apply git info and protection
	for i := range results {
		gitRoot := roots[i]
		if gitRoot == "" {
			continue
		}

		info := cache[gitRoot]

		results[i].ProjectRoot = gitRoot

		// Use the most recent of: artifact mtime, git commit time, project dir mtime
		if info.lastCommit.After(results[i].LastMod) {
			results[i].LastMod = info.lastCommit
		}
		if info.dirMtime.After(results[i].LastMod) {
			results[i].LastMod = info.dirMtime
		}

		// Mark dirty repos as protected — but only for tracked (non-gitignored) artifacts
		if info.dirty {
			ignored := ignoredByRoot[gitRoot]
			if !ignored[results[i].Path] {
				results[i].Protected = true
				results[i].Safety = model.SafetyProtected
				results[i].Reason = "uncommitted changes"
			}
		}
	}
}
