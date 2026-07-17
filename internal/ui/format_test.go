package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/ui"
)

func TestRelativeTime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"zero", time.Time{}, "unknown"},
		{"just now", time.Now().Add(-10 * time.Minute), "just now"},
		{"one hour", time.Now().Add(-90 * time.Minute), "1 hour ago"},
		{"multi hours", time.Now().Add(-5 * time.Hour), "5 hours ago"},
		{"one day", time.Now().Add(-25 * time.Hour), "1 day ago"},
		{"multi days", time.Now().Add(-3 * 24 * time.Hour), "3 days ago"},
		{"one month", time.Now().Add(-31 * 24 * time.Hour), "1 month ago"},
		{"multi months", time.Now().Add(-90 * 24 * time.Hour), "3 months ago"},
		{"one year", time.Now().Add(-400 * 24 * time.Hour), "1 year ago"},
		{"multi years", time.Now().Add(-3 * 365 * 24 * time.Hour), "3 years ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ui.RelativeTime(tt.in); got != tt.want {
				t.Errorf("RelativeTime = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusBadge(t *testing.T) {
	tests := []struct {
		in   model.ActivityStatus
		want string
	}{
		{model.StatusActive, "Active"},
		{model.StatusRecent, "Recent"},
		{model.StatusStale, "Stale"},
		{model.StatusDormant, "Dormant"},
		{model.ActivityStatus("bogus"), ""},
	}
	for _, tt := range tests {
		got := ui.StatusBadge(tt.in)
		if tt.want == "" {
			if got != "" {
				t.Errorf("StatusBadge(%v) = %q, want empty", tt.in, got)
			}
			continue
		}
		if !strings.Contains(got, tt.want) {
			t.Errorf("StatusBadge(%v) = %q, want to contain %q", tt.in, got, tt.want)
		}
	}
}

func TestSafetyIcon(t *testing.T) {
	tests := []struct {
		in   model.SafetyLevel
		want string
	}{
		{model.SafetySafe, "✔"},
		{model.SafetyCaution, "⚠"},
		{model.SafetyProtected, "✖"},
		{model.SafetyLevel("bogus"), " "},
	}
	for _, tt := range tests {
		if got := ui.SafetyIcon(tt.in); !strings.Contains(got, tt.want) {
			t.Errorf("SafetyIcon(%v) = %q, want to contain %q", tt.in, got, tt.want)
		}
	}
}
