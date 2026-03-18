package classifier

import (
	"time"

	"github.com/ohing504/devclean/internal/model"
)

// ActivityThresholds defines the day boundaries for each status.
type ActivityThresholds struct {
	Active  int // days, default 7
	Recent  int // days, default 30
	Dormant int // days, default 90
}

// DefaultThresholds returns the default activity thresholds.
func DefaultThresholds() ActivityThresholds {
	return ActivityThresholds{
		Active:  7,
		Recent:  30,
		Dormant: 90,
	}
}

// ClassifyActivity returns the activity status based on last modification time.
func ClassifyActivity(lastMod time.Time, th ActivityThresholds) model.ActivityStatus {
	if lastMod.IsZero() {
		return model.StatusDormant
	}

	days := int(time.Since(lastMod).Hours() / 24)

	switch {
	case days < th.Active:
		return model.StatusActive
	case days < th.Recent:
		return model.StatusRecent
	case days < th.Dormant:
		return model.StatusStale
	default:
		return model.StatusDormant
	}
}

// ClassifyResults applies activity classification to all scan results in place.
func ClassifyResults(results []model.ScanResult, th ActivityThresholds) {
	for i := range results {
		results[i].Activity = ClassifyActivity(results[i].LastMod, th)
	}
}
