package classifier

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// ApplyGitInfo checks git status and last commit time for each scan result.
// It marks dirty repos as protected and updates LastMod to the more recent of
// filesystem mtime vs git commit time (matching cluttered's behavior).
func ApplyGitInfo(results []model.ScanResult) {
	cache := make(map[string]*gitInfo)

	// First pass: resolve git roots and build cache
	roots := make([]string, len(results))
	// FindGitRoot forks `git rev-parse`; artifacts sharing a parent dir would
	// otherwise fork once each. Negative results ("") are cached too.
	rootByDir := make(map[string]string)
	for i := range results {
		projectDir := filepath.Dir(results[i].Path)
		gitRoot, cached := rootByDir[projectDir]
		if !cached {
			gitRoot = FindGitRoot(projectDir)
			rootByDir[projectDir] = gitRoot
		}
		// Fallback: if git root not found, walk up for topmost package.json
		if gitRoot == "" {
			gitRoot = findProjectRoot(projectDir)
		}
		if gitRoot == "" {
			gitRoot = projectDir
		}
		roots[i] = gitRoot

		if _, ok := cache[gitRoot]; !ok {
			cache[gitRoot] = &gitInfo{
				dirty:      HasUncommittedChanges(gitRoot),
				lastCommit: LastGitCommitTime(gitRoot),
				dirMtime:   pathutil.ModTime(gitRoot),
			}
		}
	}

	// Second pass: collect artifact paths per dirty git root for batch check-ignore
	dirtyRootPaths := make(map[string][]string)
	for i := range results {
		gitRoot := roots[i]
		info := cache[gitRoot]
		if info.dirty {
			dirtyRootPaths[gitRoot] = append(dirtyRootPaths[gitRoot], results[i].Path)
		}
	}

	// Batch check-ignore per dirty root
	ignoredByRoot := make(map[string]map[string]bool)
	for gitRoot, paths := range dirtyRootPaths {
		ignoredByRoot[gitRoot] = batchCheckIgnored(gitRoot, paths)
	}

	// Third pass: apply git info and protection
	for i := range results {
		gitRoot := roots[i]
		if gitRoot == "" {
			continue
		}

		info := cache[gitRoot]

		// Set project root to git root
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
