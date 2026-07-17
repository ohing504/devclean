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
