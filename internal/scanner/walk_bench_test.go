package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ohing504/devclean/internal/model"
)

// buildArtifacts materializes n artifact directories, each dirsPer subdirs of
// filesPer files, and returns ScanResults pointing at them (Size unset).
func buildArtifacts(b *testing.B, n, dirsPer, filesPer, fileSize int) []model.ScanResult {
	b.Helper()
	root := b.TempDir()
	buf := make([]byte, fileSize)
	results := make([]model.ScanResult, 0, n)
	for a := range n {
		art := filepath.Join(root, "artifact"+strconv.Itoa(a))
		for d := range dirsPer {
			dir := filepath.Join(art, "d"+strconv.Itoa(d))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				b.Fatal(err)
			}
			for f := range filesPer {
				if err := os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(f)), buf, 0o644); err != nil {
					b.Fatal(err)
				}
			}
		}
		results = append(results, model.ScanResult{Path: art})
	}
	return results
}

func benchSize(b *testing.B, workers, n, dirsPer, filesPer, fileSize int) {
	template := buildArtifacts(b, n, dirsPer, filesPer, fileSize)
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		results := make([]model.ScanResult, len(template))
		copy(results, template)
		if workers == 0 { // serial baseline
			for i := range results {
				results[i].Size = DirSize(results[i].Path)
			}
		} else {
			_ = sizePendingWorkers(ctx, results, workers)
		}
	}
}

// 40 medium artifacts (20 dirs x 50 files each) — models a real home scan with
// many node_modules/target dirs. Serial vs parallel at several pool sizes.
func BenchmarkSizeSerial(b *testing.B)     { benchSize(b, 0, 40, 20, 50, 512) }
func BenchmarkSizeParallel1(b *testing.B)  { benchSize(b, 1, 40, 20, 50, 512) }
func BenchmarkSizeParallel4(b *testing.B)  { benchSize(b, 4, 40, 20, 50, 512) }
func BenchmarkSizeParallel8(b *testing.B)  { benchSize(b, 8, 40, 20, 50, 512) }
func BenchmarkSizeParallel16(b *testing.B) { benchSize(b, 16, 40, 20, 50, 512) }
