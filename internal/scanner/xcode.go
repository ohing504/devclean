package scanner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// xcodeArtifact describes a fixed home-relative path scanned for the Xcode ecosystem.
// When expand is true, immediate child directories are reported as separate results
// (sharing the parent path as ProjectRoot for grouping). This lets users reclaim
// per-version iOS device symbols or per-device simulator data without nuking the whole tree.
//
// The kind field selects an enrichment routine that adds Label / Recommendation
// to expanded children so users can tell what each child actually is.
type xcodeArtifact struct {
	relPath     string
	category    model.Category
	safety      model.SafetyLevel
	description string
	expand      bool
	kind        xcodeKind
}

type xcodeKind int

const (
	kindGeneric xcodeKind = iota
	kindDeviceSupport
	kindSimDevice
	kindDerivedData
)

var xcodeArtifacts = []xcodeArtifact{
	{"Library/Developer/Xcode/DerivedData", model.CatBuild, model.SafetySafe, "Xcode build cache", true, kindDerivedData},
	{"Library/Developer/Xcode/Archives", model.CatBuild, model.SafetyCaution, "App archives for distribution", false, kindGeneric},
	{"Library/Developer/Xcode/iOS DeviceSupport", model.CatRuntime, model.SafetySafe, "iOS device debug symbols", true, kindDeviceSupport},
	{"Library/Developer/Xcode/watchOS DeviceSupport", model.CatRuntime, model.SafetySafe, "watchOS device debug symbols", true, kindDeviceSupport},
	{"Library/Developer/Xcode/tvOS DeviceSupport", model.CatRuntime, model.SafetySafe, "tvOS device debug symbols", true, kindDeviceSupport},
	{"Library/Developer/CoreSimulator/Devices", model.CatRuntime, model.SafetyCaution, "Simulator devices and app data", true, kindSimDevice},
	{"Library/Developer/CoreSimulator/Caches", model.CatCache, model.SafetySafe, "Simulator runtime caches", false, kindGeneric},
	{"Library/Logs/CoreSimulator", model.CatCache, model.SafetySafe, "Simulator logs", false, kindGeneric},
}

// XcodeScanner scans for Xcode and iOS Simulator caches under the user's home directory.
// It is a no-op on non-darwin platforms.
type XcodeScanner struct{}

func NewXcodeScanner() *XcodeScanner {
	return &XcodeScanner{}
}

func (s *XcodeScanner) Name() string               { return "xcode" }
func (s *XcodeScanner) Ecosystem() model.Ecosystem { return model.EcoXcode }

// VendorCleanups returns Xcode-native cleanup actions that delegate to Apple's
// own tooling. Returns nil on non-darwin so the action is never offered there.
func (s *XcodeScanner) VendorCleanups() []VendorCleanup {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return []VendorCleanup{
		{
			ID:          "simctl-delete-unavailable",
			Description: "Delete simulator devices whose runtime was removed",
			Command:     "xcrun simctl delete unavailable",
			Run: func(ctx context.Context) error {
				return exec.CommandContext(ctx, "xcrun", "simctl", "delete", "unavailable").Run()
			},
		},
	}
}

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

	// Resolve simulator UDID → metadata once (best-effort; nil on failure).
	simInfo := listSimDevices()

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
			children := expandChildren(ctx, full, a)
			enrichChildren(children, a.kind, simInfo)
			results = append(results, children...)
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

// enrichChildren attaches Label and Recommendation to expanded children based on
// the artifact kind. Mutates results in place.
func enrichChildren(results []model.ScanResult, kind xcodeKind, simInfo map[string]simDeviceInfo) {
	switch kind {
	case kindDeviceSupport:
		enrichDeviceSupport(results)
	case kindSimDevice:
		enrichSimDevices(results, simInfo)
	case kindDerivedData:
		enrichDerivedData(results)
	}
}

// enrichDeviceSupport flags older builds when the same "<device> <version>" key
// appears with multiple builds. The newest mtime per key is kept; the rest get
// a "superseded by newer build" recommendation.
//
// Folder names look like: "iPhone13,3 26.5 (23F5043k)" — model "iPhone13,3",
// version "26.5", build "23F5043k".
var deviceSupportRE = regexp.MustCompile(`^(.+) \(([^)]+)\)$`)

func enrichDeviceSupport(results []model.ScanResult) {
	type slot struct {
		idx     int
		modTime int64
	}
	newest := make(map[string]slot)

	type parsed struct {
		key     string
		matched bool
	}
	parsedAll := make([]parsed, len(results))

	for i, r := range results {
		name := filepath.Base(r.Path)
		m := deviceSupportRE.FindStringSubmatch(name)
		if len(m) != 3 {
			parsedAll[i] = parsed{matched: false}
			continue
		}
		key := m[1] // "<model> <version>"
		parsedAll[i] = parsed{key: key, matched: true}

		mt := r.LastMod.Unix()
		if cur, ok := newest[key]; !ok || mt > cur.modTime {
			newest[key] = slot{idx: i, modTime: mt}
		}
	}

	for i := range results {
		p := parsedAll[i]
		if !p.matched {
			continue
		}
		// Only flag when there are multiple builds for the same key.
		if winner := newest[p.key]; winner.idx != i {
			results[i].Recommendation = "superseded by newer build"
		}
	}
}

// enrichSimDevices uses simctl metadata to label each simulator device with
// "<name> · <runtime>" and flags devices whose runtime is no longer installed
// (isAvailable: false) as candidates for cleanup.
func enrichSimDevices(results []model.ScanResult, simInfo map[string]simDeviceInfo) {
	for i, r := range results {
		uuid := filepath.Base(r.Path)
		info, ok := simInfo[uuid]
		if !ok {
			continue
		}
		results[i].Label = info.Name + " · " + prettyRuntime(info.Runtime)
		if !info.IsAvailable {
			results[i].Recommendation = "runtime unavailable — safe to remove"
		}
	}
}

// enrichDerivedData labels well-known shared subdirectories so they are not
// confused with per-project build folders.
func enrichDerivedData(results []model.ScanResult) {
	known := map[string]string{
		"ModuleCache.noindex":      "Swift module cache (shared)",
		"SymbolCache.noindex":      "Symbol cache (shared)",
		"SDKStatCaches.noindex":    "SDK stat cache (shared)",
		"CompilationCache.noindex": "Compilation cache (shared)",
	}
	for i, r := range results {
		name := filepath.Base(r.Path)
		if label, ok := known[name]; ok {
			results[i].Label = name + " — " + label
		}
	}
}

// simDeviceInfo holds the bits of `xcrun simctl list devices` we care about.
type simDeviceInfo struct {
	Name        string
	Runtime     string
	IsAvailable bool
}

// listSimDevices runs `xcrun simctl list devices --json` and returns UDID→info.
// Returns nil on any failure (xcrun not installed, malformed JSON, etc.) so the
// caller can fall back to UUID-only display.
func listSimDevices() map[string]simDeviceInfo {
	out, err := exec.Command("xcrun", "simctl", "list", "devices", "--json").Output()
	if err != nil {
		return nil
	}
	var raw struct {
		Devices map[string][]struct {
			UDID        string `json:"udid"`
			Name        string `json:"name"`
			IsAvailable bool   `json:"isAvailable"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}
	result := make(map[string]simDeviceInfo)
	for rt, devices := range raw.Devices {
		short := strings.TrimPrefix(rt, "com.apple.CoreSimulator.SimRuntime.")
		for _, d := range devices {
			result[d.UDID] = simDeviceInfo{
				Name:        d.Name,
				Runtime:     short,
				IsAvailable: d.IsAvailable,
			}
		}
	}
	return result
}

// prettyRuntime turns "iOS-26-3" into "iOS 26.3".
func prettyRuntime(rt string) string {
	parts := strings.Split(rt, "-")
	if len(parts) < 2 {
		return rt
	}
	return parts[0] + " " + strings.Join(parts[1:], ".")
}

// isUnderRoot reports whether path is the same as root or a descendant of it.
func isUnderRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
