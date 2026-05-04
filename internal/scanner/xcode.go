package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// xcodeArtifact describes a fixed home-relative path scanned for the Xcode ecosystem.
// When expand is true, immediate child directories are reported as separate results
// (sharing the parent path as ProjectRoot for grouping). This lets users reclaim
// per-version iOS device symbols or per-device simulator data without nuking the whole tree.
type xcodeArtifact struct {
	relPath     string
	category    model.Category
	safety      model.SafetyLevel
	description string
	expand      bool
}

var xcodeArtifacts = []xcodeArtifact{
	{"Library/Developer/Xcode/DerivedData", model.CatBuild, model.SafetySafe, "Xcode build cache", false},
	{"Library/Developer/Xcode/Archives", model.CatBuild, model.SafetyCaution, "App archives for distribution", false},
	{"Library/Developer/Xcode/iOS DeviceSupport", model.CatRuntime, model.SafetySafe, "iOS device debug symbols", true},
	{"Library/Developer/Xcode/watchOS DeviceSupport", model.CatRuntime, model.SafetySafe, "watchOS device debug symbols", true},
	{"Library/Developer/Xcode/tvOS DeviceSupport", model.CatRuntime, model.SafetySafe, "tvOS device debug symbols", true},
	{"Library/Developer/CoreSimulator/Devices", model.CatRuntime, model.SafetyCaution, "Simulator devices and app data", true},
	{"Library/Developer/CoreSimulator/Caches", model.CatCache, model.SafetySafe, "Simulator runtime caches", false},
	{"Library/Logs/CoreSimulator", model.CatCache, model.SafetySafe, "Simulator logs", false},
}

// XcodeScanner scans for Xcode and iOS Simulator caches under the user's home directory.
// It is a no-op on non-darwin platforms.
type XcodeScanner struct{}

func NewXcodeScanner() *XcodeScanner {
	return &XcodeScanner{}
}

func (s *XcodeScanner) Name() string               { return "xcode" }
func (s *XcodeScanner) Ecosystem() model.Ecosystem { return model.EcoXcode }

func (s *XcodeScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	var results []model.ScanResult
	for _, a := range xcodeArtifacts {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		full := filepath.Join(home, a.relPath)
		if !isUnderRoot(full, absRoot) {
			continue
		}
		info, err := os.Stat(full)
		if err != nil || !info.IsDir() {
			continue
		}

		if a.expand {
			results = append(results, expandChildren(ctx, full, a)...)
			continue
		}

		size := DirSize(full)
		results = append(results, model.ScanResult{
			Path:        full,
			Ecosystem:   model.EcoXcode,
			Category:    a.category,
			Size:        size,
			LastMod:     ModTime(full),
			Safety:      a.safety,
			ProjectRoot: filepath.Dir(full),
		})
		ReportProgress(ctx, len(results))
	}
	return results, nil
}

// expandChildren reports each immediate subdirectory of parent as its own ScanResult,
// grouped together via a shared ProjectRoot. Empty parents yield no results.
func expandChildren(ctx context.Context, parent string, a xcodeArtifact) []model.ScanResult {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil
	}
	var out []model.ScanResult
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		select {
		case <-ctx.Done():
			return out
		default:
		}
		child := filepath.Join(parent, e.Name())
		out = append(out, model.ScanResult{
			Path:        child,
			Ecosystem:   model.EcoXcode,
			Category:    a.category,
			Size:        DirSize(child),
			LastMod:     ModTime(child),
			Safety:      a.safety,
			ProjectRoot: parent,
		})
		ReportProgress(ctx, len(out))
	}
	return out
}

// isUnderRoot reports whether path is the same as root or a descendant of it.
func isUnderRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}
