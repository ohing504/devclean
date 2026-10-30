package output

import (
	"encoding/json"
	"io"

	"github.com/ohing504/devclean/internal/model"
)

// ScanOutput is the top-level JSON structure for scan results.
type ScanOutput struct {
	TotalSize  int64              `json:"total_size"`
	TotalCount int                `json:"total_count"`
	Results    []model.ScanResult `json:"results"`
}

// WriteJSON writes scan results as formatted JSON.
func WriteJSON(w io.Writer, results []model.ScanResult) error {
	out := ScanOutput{
		TotalSize:  model.DedupedTotal(results), // nets out blocks shared via hard links
		TotalCount: len(results),
		Results:    results,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// Clean item results, as in CleanItem.Result.
const (
	CleanTrashed    = "trashed"     // moved to the Trash: the space is freed when it is emptied
	CleanFreed      = "freed"       // deleted permanently, or reclaimed by its DeleteMethod
	CleanFailed     = "failed"      // not removed; Error says why
	CleanWouldClean = "would_clean" // --dry-run: a real run would remove it
	CleanWouldFail  = "would_fail"  // --dry-run: a real run would refuse it; Error says why
)

// CleanOutput is the top-level JSON structure for clean results.
type CleanOutput struct {
	DryRun bool `json:"dry_run"`
	// TrashedSize and FreedSize total the items moved to the Trash and the
	// items freed, each with blocks shared by hard links counted once.
	TrashedSize    int64          `json:"trashed_size"`
	FreedSize      int64          `json:"freed_size"`
	Results        []CleanItem    `json:"results"`
	Skipped        []SkippedItem  `json:"skipped,omitempty"`
	VendorCleanups []VendorResult `json:"vendor_cleanups,omitempty"`
}

// CleanItem is one artifact clean tried, or would try under --dry-run.
type CleanItem struct {
	Path      string          `json:"path"`
	Ecosystem model.Ecosystem `json:"ecosystem"`
	Size      int64           `json:"size"`
	Label     string          `json:"label,omitempty"`
	Result    string          `json:"result"`
	Error     string          `json:"error,omitempty"`
}

// SkippedItem is an artifact the --yes safety gate kept: Reason is "caution"
// or "protected".
type SkippedItem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// VendorResult is one --vendor-cleanup command: Result is "ran", "failed"
// (Error says why) or, under --dry-run, "would_run".
type VendorResult struct {
	Ecosystem   string `json:"ecosystem"`
	Description string `json:"description"`
	Command     string `json:"command"`
	Result      string `json:"result"`
	Error       string `json:"error,omitempty"`
}

// WriteCleanJSON writes clean results as formatted JSON.
func WriteCleanJSON(w io.Writer, out CleanOutput) error {
	if out.Results == nil {
		out.Results = []CleanItem{} // an empty list, not null
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
