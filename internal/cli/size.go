package cli

import (
	"fmt"

	"github.com/dustin/go-humanize"
)

// parseMinSize parses a human-friendly size string like "1MB", "500KB" or "2.5GB"
// into bytes. Empty strings return (0, nil) meaning "no filter". Negative or
// out-of-range values are rejected.
func parseMinSize(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	n, err := humanize.ParseBytes(s)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	if n > 1<<62 {
		return 0, fmt.Errorf("size %q out of range", s)
	}
	return int64(n), nil
}
