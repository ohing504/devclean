package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/output"
)

func sampleResults() []model.ScanResult {
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []model.ScanResult{
		{
			Path:      "/Users/dev/project/node_modules",
			Ecosystem: model.EcoNode,
			Category:  model.CatDeps,
			Size:      104857600, // 100 MB
			LastMod:   fixedTime,
			Activity:  model.StatusDormant,
			Safety:    model.SafetySafe,
		},
		{
			Path:      "/Users/dev/ios/DerivedData",
			Ecosystem: model.EcoXcode,
			Category:  model.CatBuild,
			Size:      524288000, // 500 MB
			LastMod:   fixedTime,
			Activity:  model.StatusStale,
			Safety:    model.SafetyCaution,
		},
	}
}

// TestWriteTableSparseAnnotation verifies a materially sparse artifact renders
// its apparent size alongside disk — the point of A1. Full render-path test:
// it pins that the artifact size cell actually routes through sizeCell.
func TestWriteTableSparseAnnotation(t *testing.T) {
	sparse := model.ScanResult{
		Path:         "/Users/dev/proj/node_modules",
		Ecosystem:    model.EcoNode,
		Category:     model.CatDeps,
		ProjectRoot:  "/Users/dev/proj",
		Size:         4096,       // disk: sparse, almost nothing allocated
		ApparentSize: 2147483649, // apparent: ~2 GiB logical
		Activity:     model.StatusDormant,
		Safety:       model.SafetySafe,
	}
	var buf bytes.Buffer
	output.WriteTableWithOptions(&buf, []model.ScanResult{sparse}, output.TableOptions{Verbose: true})
	if out := buf.String(); !strings.Contains(out, "apparent") {
		t.Errorf("sparse artifact must show apparent size; got:\n%s", out)
	}
}

// TestWriteTableNoSparseAnnotationForDense verifies a normal artifact (apparent
// below disk from block slack) is not annotated — the threshold must not fire
// on ordinary trees.
func TestWriteTableNoSparseAnnotationForDense(t *testing.T) {
	dense := model.ScanResult{
		Path:         "/Users/dev/proj/node_modules",
		Ecosystem:    model.EcoNode,
		Category:     model.CatDeps,
		ProjectRoot:  "/Users/dev/proj",
		Size:         150 * 1000 * 1000, // 150 MB disk
		ApparentSize: 122 * 1000 * 1000, // apparent below disk (rounding slack)
		Activity:     model.StatusDormant,
		Safety:       model.SafetySafe,
	}
	var buf bytes.Buffer
	output.WriteTableWithOptions(&buf, []model.ScanResult{dense}, output.TableOptions{Verbose: true})
	if out := buf.String(); strings.Contains(out, "apparent") {
		t.Errorf("dense artifact must not be annotated; got:\n%s", out)
	}
}

// TestWriteJSONDedupsHardlinkedTotal pins that the JSON total_size nets out
// blocks shared across artifacts via hard links (the pnpm case), rather than
// summing the overlapping per-artifact sizes.
func TestWriteJSONDedupsHardlinkedTotal(t *testing.T) {
	shared := map[model.InodeKey]int64{{Dev: 1, Ino: 7}: 400}
	results := []model.ScanResult{
		{Path: "/a", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 500, Links: shared},
		{Path: "/b", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 500, Links: shared},
	}
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, results); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}
	var out output.ScanOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	// 500 + 500 − 400 (shared inode counted once) = 600, not 1000.
	if out.TotalSize != 600 {
		t.Errorf("TotalSize = %d, want 600 (deduped)", out.TotalSize)
	}
}

// TestWriteTableDedupNote verifies the table annotates its grand total when
// hard-link dedup made it smaller than the naive sum.
func TestWriteTableDedupNote(t *testing.T) {
	shared := map[model.InodeKey]int64{{Dev: 1, Ino: 7}: 400}
	results := []model.ScanResult{
		{Path: "/a/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, ProjectRoot: "/a", Size: 500, Links: shared, Safety: model.SafetySafe, Activity: model.StatusDormant},
		{Path: "/b/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, ProjectRoot: "/b", Size: 500, Links: shared, Safety: model.SafetySafe, Activity: model.StatusDormant},
	}
	var buf bytes.Buffer
	output.WriteTableWithOptions(&buf, results, output.TableOptions{Verbose: true})
	if out := buf.String(); !strings.Contains(out, "excludes hard-linked") {
		t.Errorf("expected hard-link dedup note in total; got:\n%s", out)
	}
}

func TestWriteJSON(t *testing.T) {
	results := sampleResults()
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, results); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	var out output.ScanOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if out.TotalCount != 2 {
		t.Errorf("expected TotalCount=2, got %d", out.TotalCount)
	}
	expectedSize := int64(104857600 + 524288000)
	if out.TotalSize != expectedSize {
		t.Errorf("expected TotalSize=%d, got %d", expectedSize, out.TotalSize)
	}
	if len(out.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(out.Results))
	}
}

func TestWriteJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := output.WriteJSON(&buf, nil); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	var out output.ScanOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	if out.TotalCount != 0 {
		t.Errorf("expected TotalCount=0, got %d", out.TotalCount)
	}
}

func TestWriteTable(t *testing.T) {
	results := sampleResults()
	var buf bytes.Buffer
	output.WriteTable(&buf, results)

	out := buf.String()

	// Xcode (500 MB) should appear before Node (100 MB)
	xcodeIdx := strings.Index(out, "xcode")
	nodeIdx := strings.Index(out, "node")
	if xcodeIdx == -1 || nodeIdx == -1 {
		t.Fatalf("expected both ecosystems in output:\n%s", out)
	}
	if xcodeIdx > nodeIdx {
		t.Errorf("expected xcode (larger) before node (smaller)")
	}

	if !strings.Contains(out, "Total") {
		t.Error("expected Total summary in output")
	}
}

func TestWriteTable_Legend(t *testing.T) {
	results := sampleResults()
	var buf bytes.Buffer
	output.WriteTable(&buf, results)
	out := buf.String()

	if !strings.Contains(out, "safe") && !strings.Contains(out, "caution") {
		t.Errorf("expected legend with safety level descriptions:\n%s", out)
	}
	if !strings.Contains(out, "Active") && !strings.Contains(out, "Dormant") {
		t.Errorf("expected legend with activity status descriptions:\n%s", out)
	}
}

func TestWriteTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	output.WriteTable(&buf, nil)

	if !strings.Contains(buf.String(), "No items found") {
		t.Error("expected 'No items found' for empty results")
	}
}

func TestWriteTable_LastUsedTag(t *testing.T) {
	// Artifacts with a non-zero LastUsedAt get a "last used …" tag; artifacts
	// without one render exactly as before (no tag).
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []model.ScanResult{
		{Path: "/home/.lmstudio/models/org/model", Ecosystem: model.EcoLLM, Category: model.CatCache, Size: 500000000, LastMod: fixedTime, Safety: model.SafetySafe, LastUsedAt: fixedTime},
		{Path: "/home/.npm", Ecosystem: model.EcoGlobal, Category: model.CatCache, Size: 300000000, LastMod: fixedTime, Safety: model.SafetySafe},
	}

	var buf bytes.Buffer
	output.WriteTable(&buf, results)
	out := buf.String()

	if !strings.Contains(out, "last used") {
		t.Errorf("expected 'last used' tag for non-zero LastUsedAt:\n%s", out)
	}
	// Only the LLM artifact carries the tag — the zero-value one must not.
	if strings.Count(out, "last used") != 1 {
		t.Errorf("expected exactly 1 'last used' tag (zero LastUsedAt renders nothing):\n%s", out)
	}
}

func TestWriteTable_TopNByProject(t *testing.T) {
	// --top should limit by project count, not individual artifact count.
	// A project with 2 artifacts should count as 1 project.
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []model.ScanResult{
		// Project A: 2 artifacts, total 500 MB
		{Path: "/a/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 300000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/a"},
		{Path: "/a/.next", Ecosystem: model.EcoNode, Category: model.CatBuild, Size: 200000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/a"},
		// Project B: 1 artifact, 400 MB
		{Path: "/b/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 400000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/b"},
		// Project C: 1 artifact, 100 MB
		{Path: "/c/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 100000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/c"},
	}

	var buf bytes.Buffer
	output.WriteTableWithOptions(&buf, results, output.TableOptions{TopN: 2})
	out := buf.String()

	// Top 2 by project size: A (500MB) and B (400MB). C should be excluded.
	if strings.Contains(out, "/c\n") {
		t.Errorf("project C should not appear in top 2:\n%s", out)
	}
	// Should show 2 projects, not 3
	if strings.Contains(out, "3 projects") {
		t.Errorf("expected 2 projects, got 3:\n%s", out)
	}
	// A should be present with both artifacts
	if !strings.Contains(out, ".next") {
		t.Errorf("project A should have both artifacts:\n%s", out)
	}
}

func TestWriteTable_MonorepoRelativePaths(t *testing.T) {
	// Monorepo artifacts should be grouped by sub-package with headers.
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []model.ScanResult{
		{Path: "/monorepo/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 500000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/monorepo"},
		{Path: "/monorepo/apps/web/.next", Ecosystem: model.EcoNode, Category: model.CatBuild, Size: 300000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/monorepo"},
		{Path: "/monorepo/apps/admin/.next", Ecosystem: model.EcoNode, Category: model.CatBuild, Size: 200000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/monorepo"},
	}

	var buf bytes.Buffer
	output.WriteTableWithOptions(&buf, results, output.TableOptions{Verbose: true})
	out := buf.String()

	// Sub-package headers should appear
	if !strings.Contains(out, "apps/web (") {
		t.Errorf("expected sub-package header 'apps/web (':\n%s", out)
	}
	if !strings.Contains(out, "apps/admin (") {
		t.Errorf("expected sub-package header 'apps/admin (':\n%s", out)
	}
	// Artifacts show just basename under sub-package
	if !strings.Contains(out, "node_modules") {
		t.Errorf("expected 'node_modules':\n%s", out)
	}
	if !strings.Contains(out, ".next") {
		t.Errorf("expected '.next':\n%s", out)
	}
}

func TestWriteTable_SubPackageGrouping(t *testing.T) {
	// Monorepo artifacts should be grouped by sub-package.
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []model.ScanResult{
		{Path: "/mono/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 500000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/.turbo", Ecosystem: model.EcoNode, Category: model.CatCache, Size: 200000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/apps/web/.next", Ecosystem: model.EcoNode, Category: model.CatBuild, Size: 300000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/apps/web/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 10000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/apps/admin/.next", Ecosystem: model.EcoNode, Category: model.CatBuild, Size: 200000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
	}

	var buf bytes.Buffer
	output.WriteTableWithOptions(&buf, results, output.TableOptions{Verbose: true})
	out := buf.String()

	// Should show sub-package headers with size totals (not just artifact relative paths)
	// Root group should exist
	if !strings.Contains(out, ". (") {
		t.Errorf("expected root sub-package header '. (':\n%s", out)
	}
	// apps/web group header with size
	if !strings.Contains(out, "apps/web (") {
		t.Errorf("expected sub-package header 'apps/web (':\n%s", out)
	}
	// apps/admin group header with size
	if !strings.Contains(out, "apps/admin (") {
		t.Errorf("expected sub-package header 'apps/admin (':\n%s", out)
	}
	// Artifacts should show just the filename, not the full relative path
	if !strings.Contains(out, ".next") {
		t.Errorf("expected artifact '.next':\n%s", out)
	}
}

func TestWriteTable_DefaultCollapseSmallPackages(t *testing.T) {
	// Default mode should collapse small sub-packages in monorepos.
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []model.ScanResult{
		{Path: "/mono/node_modules", Ecosystem: model.EcoNode, Category: model.CatDeps, Size: 500000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/.turbo", Ecosystem: model.EcoNode, Category: model.CatCache, Size: 200000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/apps/web/.next", Ecosystem: model.EcoNode, Category: model.CatBuild, Size: 300000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/apps/admin/.next", Ecosystem: model.EcoNode, Category: model.CatBuild, Size: 100000000, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/packages/ui/.turbo", Ecosystem: model.EcoNode, Category: model.CatCache, Size: 100, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
		{Path: "/mono/packages/db/.turbo", Ecosystem: model.EcoNode, Category: model.CatCache, Size: 50, LastMod: fixedTime, Safety: model.SafetySafe, ProjectRoot: "/mono"},
	}

	var buf bytes.Buffer
	output.WriteTable(&buf, results)
	out := buf.String()

	// Small sub-packages (packages/ui, packages/db) should be collapsed
	if !strings.Contains(out, "more packages") {
		t.Errorf("expected small packages collapsed with 'more packages', got:\n%s", out)
	}
	// Large sub-packages should still show
	if !strings.Contains(out, "apps/web") {
		t.Errorf("expected apps/web to show, got:\n%s", out)
	}
}

func TestWriteTable_MonorepoGrouping(t *testing.T) {
	// Monorepo: artifacts spread across sub-packages should be grouped
	// under one project when they share the same ProjectRoot (git root).
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []model.ScanResult{
		{
			Path:        "/Users/dev/monorepo/node_modules",
			Ecosystem:   model.EcoNode,
			Category:    model.CatDeps,
			Size:        500000000,
			LastMod:     fixedTime,
			Safety:      model.SafetySafe,
			ProjectRoot: "/Users/dev/monorepo",
		},
		{
			Path:        "/Users/dev/monorepo/.turbo",
			Ecosystem:   model.EcoNode,
			Category:    model.CatCache,
			Size:        200000000,
			LastMod:     fixedTime,
			Safety:      model.SafetySafe,
			ProjectRoot: "/Users/dev/monorepo",
		},
		{
			Path:        "/Users/dev/monorepo/apps/web/.next",
			Ecosystem:   model.EcoNode,
			Category:    model.CatBuild,
			Size:        300000000,
			LastMod:     fixedTime,
			Safety:      model.SafetySafe,
			ProjectRoot: "/Users/dev/monorepo",
		},
		{
			Path:        "/Users/dev/monorepo/apps/web/node_modules",
			Ecosystem:   model.EcoNode,
			Category:    model.CatDeps,
			Size:        5000,
			LastMod:     fixedTime,
			Safety:      model.SafetySafe,
			ProjectRoot: "/Users/dev/monorepo",
		},
	}

	var buf bytes.Buffer
	output.WriteTable(&buf, results)
	out := buf.String()

	// All 4 items share ProjectRoot, so should be grouped as 1 project
	if strings.Count(out, "1 projects") != 1 {
		t.Errorf("expected 1 project group for monorepo, got:\n%s", out)
	}

	// Total should reflect all items
	if !strings.Contains(out, "4 items") {
		t.Errorf("expected 4 items in total, got:\n%s", out)
	}
}
