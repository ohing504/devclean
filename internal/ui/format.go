package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ohing504/devclean/internal/model"
)

// StatusBadge renders an activity status as a colored label ("" for unknown).
func StatusBadge(s model.ActivityStatus) string {
	switch s {
	case model.StatusActive:
		return ActiveStyle.Render("Active")
	case model.StatusRecent:
		return RecentStyle.Render("Recent")
	case model.StatusStale:
		return StaleStyle.Render("Stale")
	case model.StatusDormant:
		return DormantStyle.Render("Dormant")
	default:
		return ""
	}
}

// SafetyIcon renders a safety level as a colored glyph (a blank for unknown).
func SafetyIcon(s model.SafetyLevel) string {
	switch s {
	case model.SafetySafe:
		return SafeStyle.Render("✔")
	case model.SafetyCaution:
		return CautionStyle.Render("⚠")
	case model.SafetyProtected:
		return ProtectedStyle.Render("✖")
	default:
		return " "
	}
}

// RelativeTime formats t as a coarse "N units ago" string, or "unknown" for the
// zero time.
func RelativeTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}

	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / 24 / 30)
		if months <= 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(d.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// Artifact cells shared by every view that lists artifacts (scan table, clean
// selector), so the same artifact reads the same everywhere.

// ArtifactName returns the scanner-provided Label when set, else the artifact
// path relative to root, else its basename (root empty or not an ancestor).
func ArtifactName(r model.ScanResult, root string) string {
	if r.Label != "" {
		return r.Label
	}
	if root != "" {
		rel, err := filepath.Rel(root, r.Path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return rel
		}
	}
	return filepath.Base(r.Path)
}

// sparseMinDiff is the apparent−disk gap above which a size is worth annotating
// as sparse. Below it, ordinary block-rounding slack (apparent can even fall
// under disk) would produce noise.
const sparseMinDiff = 1 << 30 // 1 GiB

// sharedMinDiff is the AllocatedSize−Size gap worth annotating.
const sharedMinDiff = 100 * 1000 * 1000 // 100 MB

// ArtifactSize renders an artifact's real on-disk size, annotating it with the
// larger size the file nominally reports when it is materially sparse —
// nominal more than double disk and over sparseMinDiff larger. Example: a
// Docker.raw image shows "24.0 GB (appears as 460.0 GB)", making clear it only
// uses 24 GB on disk though it presents itself as 460 GB.
// Clone-aware sizes annotate their allocated blocks instead, e.g.
// "760.0 MB (11.3 GB incl. shared blocks)".
func ArtifactSize(r model.ScanResult) string {
	if r.AllocatedSize-r.Size > sharedMinDiff {
		return fmt.Sprintf("%s (%s incl. shared blocks)", model.HumanSize(r.Size), model.HumanSize(r.AllocatedSize))
	}
	if r.ApparentSize > r.Size*2 && r.ApparentSize-r.Size > sparseMinDiff {
		return fmt.Sprintf("%s (appears as %s)", model.HumanSize(r.Size), model.HumanSize(r.ApparentSize))
	}
	return model.HumanSize(r.Size)
}

// LastUsedTag formats r.LastUsedAt as a dim trailing tag, or "" when the
// scanner did not populate it.
func LastUsedTag(r model.ScanResult) string {
	if r.LastUsedAt.IsZero() {
		return ""
	}
	return " " + DimStyle.Render("· last used "+RelativeTime(r.LastUsedAt))
}

// RecommendationTag formats r.Recommendation as a styled trailing tag, or "" if empty.
func RecommendationTag(r model.ScanResult) string {
	if r.Recommendation == "" {
		return ""
	}
	return "  " + RecommendStyle.Render("← "+r.Recommendation)
}
