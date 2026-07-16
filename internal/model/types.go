package model

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// Ecosystem identifies a development ecosystem.
type Ecosystem string

const (
	EcoXcode   Ecosystem = "xcode"
	EcoAndroid Ecosystem = "android"
	EcoFlutter Ecosystem = "flutter"
	EcoNode    Ecosystem = "node"
	EcoDocker  Ecosystem = "docker"
	EcoPython  Ecosystem = "python"
	EcoRust    Ecosystem = "rust"
	EcoRuby    Ecosystem = "ruby"
	EcoGo      Ecosystem = "go"
	EcoGlobal  Ecosystem = "global"
	EcoLLM     Ecosystem = "llm"
)

// AllEcosystems returns all supported ecosystems in display order.
func AllEcosystems() []Ecosystem {
	return []Ecosystem{
		EcoXcode, EcoAndroid, EcoFlutter,
		EcoNode, EcoDocker, EcoPython, EcoRust, EcoRuby, EcoGo, EcoGlobal, EcoLLM,
	}
}

// Category classifies the type of artifact.
type Category string

const (
	CatCache   Category = "cache"
	CatBuild   Category = "build"
	CatRuntime Category = "runtime"
	CatDeps    Category = "deps"
)

// AllCategories returns all supported categories.
func AllCategories() []Category {
	return []Category{CatCache, CatBuild, CatRuntime, CatDeps}
}

// ActivityStatus indicates how recently an item was used.
type ActivityStatus string

const (
	StatusActive  ActivityStatus = "active"  // < 7 days
	StatusRecent  ActivityStatus = "recent"  // 7–30 days
	StatusStale   ActivityStatus = "stale"   // 30–90 days
	StatusDormant ActivityStatus = "dormant" // 90+ days
)

// SafetyLevel indicates how safe it is to delete an item.
type SafetyLevel string

const (
	SafetySafe      SafetyLevel = "safe"      // freely deletable, auto-regenerated
	SafetyCaution   SafetyLevel = "caution"   // deletable but may need rebuild or has shared impact
	SafetyProtected SafetyLevel = "protected" // should not be deleted
)

// ArtifactDef defines a cleanable artifact pattern for an ecosystem.
type ArtifactDef struct {
	Pattern     string   `json:"pattern"`
	Category    Category `json:"category"`
	Description string   `json:"description"`
	AlwaysSafe  bool     `json:"always_safe"`
}

// ScanResult represents a single scannable item on disk.
type ScanResult struct {
	Path           string         `json:"path"`
	Ecosystem      Ecosystem      `json:"ecosystem"`
	Category       Category       `json:"category"`
	Size           int64          `json:"size"`
	LastMod        time.Time      `json:"last_modified"`
	Activity       ActivityStatus `json:"activity"`
	Safety         SafetyLevel    `json:"safety"`
	Protected      bool           `json:"protected"`
	Reason         string         `json:"reason,omitempty"`
	ProjectRoot    string         `json:"project_root,omitempty"`
	Label          string         `json:"label,omitempty"`          // human-readable display name (e.g. "iPhone 17 Pro · iOS 26.3")
	Recommendation string         `json:"recommendation,omitempty"` // hint for the user (e.g. "old build", "unavailable runtime")
	LastUsedAt     time.Time      `json:"last_used_at,omitzero"`    // when the item itself was last used, if the scanner can tell (omitzero: zero time is dropped from JSON)
}

// HumanSize returns a human-readable size string.
func (r ScanResult) HumanSize() string {
	return HumanSize(r.Size)
}

// HumanSize formats bytes into a human-readable string using decimal SI
// units (1 KB = 1000 B). This matches the macOS Finder convention and keeps
// the display aligned with the `--min-size` parser, which treats "MB" as
// 10^6 per humanize/SI convention. Internally we still derive sizes from
// `du -sk` (binary kilobytes), but the formatting layer is decimal so the
// number a user types and the number they see agree on the same threshold.
func HumanSize(size int64) string {
	const (
		KB = 1000
		MB = KB * 1000
		GB = MB * 1000
		TB = GB * 1000
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.1f TB", float64(size)/float64(TB))
	case size >= GB:
		return fmt.Sprintf("%.1f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.1f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// ProtectionResult holds the result of a protection analysis.
type ProtectionResult struct {
	IsProtected bool     `json:"is_protected"`
	Reasons     []string `json:"reasons"`
}

// ProjectKey returns the grouping key for this scan result.
func (r ScanResult) ProjectKey() string {
	if r.ProjectRoot != "" {
		return r.ProjectRoot
	}
	return filepath.Dir(r.Path)
}

// FilterResults returns a new slice containing only results for which pred returns true.
func FilterResults(results []ScanResult, pred func(ScanResult) bool) []ScanResult {
	var filtered []ScanResult
	for _, r := range results {
		if pred(r) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// ProjectGroup groups scan results that belong to the same project.
type ProjectGroup struct {
	Name      string
	Path      string
	TotalSize int64
	LastMod   time.Time
	Activity  ActivityStatus
	Protected bool
	Items     []ScanResult
}

// GroupByProject groups scan results by their project root, sorted by total size descending.
func GroupByProject(results []ScanResult) []ProjectGroup {
	m := make(map[string]*ProjectGroup)
	for _, r := range results {
		key := r.ProjectKey()
		p, ok := m[key]
		if !ok {
			p = &ProjectGroup{
				Name: filepath.Base(key),
				Path: key,
			}
			m[key] = p
		}
		p.TotalSize += r.Size
		p.Items = append(p.Items, r)
		if r.LastMod.After(p.LastMod) {
			p.LastMod = r.LastMod
			p.Activity = r.Activity
		}
		if r.Protected {
			p.Protected = true
		}
	}
	var groups []ProjectGroup
	for _, g := range m {
		sort.Slice(g.Items, func(i, j int) bool {
			return g.Items[i].Size > g.Items[j].Size
		})
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].TotalSize > groups[j].TotalSize
	})
	return groups
}
