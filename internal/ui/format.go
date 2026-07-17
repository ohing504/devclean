package ui

import (
	"fmt"
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
