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
	var totalSize int64
	for _, r := range results {
		totalSize += r.Size
	}

	out := ScanOutput{
		TotalSize:  totalSize,
		TotalCount: len(results),
		Results:    results,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
