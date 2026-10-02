package pathutil

import (
	"os"
	"strings"
	"sync"
	"time"
)

var (
	homeDir     string
	homeDirOnce sync.Once
)

// CachedHomeDir returns the user's home directory, cached after first call.
func CachedHomeDir() string {
	homeDirOnce.Do(func() { homeDir, _ = os.UserHomeDir() })
	return homeDir
}

// ShortenHome replaces the home directory prefix with "~".
func ShortenHome(path string) string {
	home := CachedHomeDir()
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(os.PathSeparator)); ok {
		return "~" + string(os.PathSeparator) + rest
	}
	return path
}

// ModTime returns the modification time of a path, or zero time on error.
func ModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
