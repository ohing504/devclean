package scanner_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohing504/devclean/internal/model"
	"github.com/ohing504/devclean/internal/scanner"
)

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// artifact is the comparable identity of one walk result. Paths are relative
// to the scan root; Root is ProjectRoot, empty for ecosystems that don't set it.
type artifact struct {
	Path     string
	Eco      model.Ecosystem
	Category model.Category
	Safety   model.SafetyLevel
	Root     string
}

func (a artifact) String() string {
	s := fmt.Sprintf("%s [%s %s %s]", a.Path, a.Eco, a.Category, a.Safety)
	if a.Root != "" {
		s += " root=" + a.Root
	}
	return s
}

// walkCase is one WalkScan scenario: build tree under a temp root, scan it
// with ecos, and expect exactly want — no missing and no extra artifacts.
type walkCase struct {
	name string
	ecos []model.Ecosystem
	// tree lists paths relative to the root; a trailing "/" makes a directory,
	// anything else an empty file (parents are created as needed).
	tree []string
	want []artifact
}

func runWalkCases(t *testing.T, cases []walkCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, p := range tc.tree {
				full := filepath.Join(root, filepath.FromSlash(p))
				if strings.HasSuffix(p, "/") {
					mustMkdir(t, full)
					continue
				}
				mustMkdir(t, filepath.Dir(full))
				mustWriteFile(t, full, nil)
			}

			results, err := scanner.WalkScan(context.Background(), root, tc.ecos...)
			if err != nil {
				t.Fatalf("WalkScan: %v", err)
			}
			assertArtifacts(t, root, results, tc.want)
		})
	}
}

func assertArtifacts(t *testing.T, root string, results []model.ScanResult, want []artifact) {
	t.Helper()
	rel := func(p string) string {
		if p == "" {
			return ""
		}
		r, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatalf("rel %s: %v", p, err)
		}
		return filepath.ToSlash(r)
	}

	// Counted, not a set: a duplicate double-counts reclaimable size.
	got := make(map[artifact]int, len(results))
	for _, r := range results {
		got[artifact{rel(r.Path), r.Ecosystem, r.Category, r.Safety, rel(r.ProjectRoot)}]++
	}
	for _, w := range want {
		if got[w] == 0 {
			t.Errorf("missing:    %s", w)
			continue
		}
		got[w]--
	}
	for a, n := range got {
		for range n {
			t.Errorf("unexpected: %s", a)
		}
	}
}

type fakeScanner struct {
	name      string
	ecosystem model.Ecosystem
	results   []model.ScanResult
}

func (f *fakeScanner) Name() string               { return f.name }
func (f *fakeScanner) Ecosystem() model.Ecosystem { return f.ecosystem }

func (f *fakeScanner) Scan(_ context.Context, _ string) ([]model.ScanResult, error) {
	return f.results, nil
}

// Order matters: it decides attribution when rules of several ecosystems match.
func TestDefaultRegistry(t *testing.T) {
	want := []struct {
		name string
		eco  model.Ecosystem
	}{
		{"node", model.EcoNode},
		{"rust", model.EcoRust},
		{"ruby", model.EcoRuby},
		{"python", model.EcoPython},
		{"go", model.EcoGo},
		{"flutter", model.EcoFlutter},
		{"android", model.EcoAndroid},
		{"xcode", model.EcoXcode},
		{"docker", model.EcoDocker},
		{"global", model.EcoGlobal},
		{"llm", model.EcoLLM},
	}

	got := scanner.DefaultRegistry().All()
	if len(got) != len(want) {
		t.Fatalf("registered %d scanners, want %d", len(got), len(want))
	}
	for i, s := range got {
		if s.Name() != want[i].name || s.Ecosystem() != want[i].eco {
			t.Errorf("scanner %d = %s/%s, want %s/%s", i, s.Name(), s.Ecosystem(), want[i].name, want[i].eco)
		}
	}
}

func TestRegistryScanAll(t *testing.T) {
	reg := scanner.NewRegistry()
	reg.Register(&fakeScanner{
		name:      "node",
		ecosystem: model.EcoNode,
		results: []model.ScanResult{
			{Path: "/project/node_modules", Ecosystem: model.EcoNode, Size: 1024},
		},
	})
	reg.Register(&fakeScanner{
		name:      "python",
		ecosystem: model.EcoPython,
		results: []model.ScanResult{
			{Path: "/project/venv", Ecosystem: model.EcoPython, Size: 2048},
		},
	})

	results, err := reg.ScanAll(context.Background(), "/")
	if err != nil {
		t.Fatalf("ScanAll error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Ecosystem != model.EcoNode {
		t.Errorf("expected first result=node, got %s", results[0].Ecosystem)
	}
	if results[1].Ecosystem != model.EcoPython {
		t.Errorf("expected second result=python, got %s", results[1].Ecosystem)
	}
}

func TestRegistryForEcosystems(t *testing.T) {
	reg := scanner.NewRegistry()
	reg.Register(&fakeScanner{name: "node", ecosystem: model.EcoNode})
	reg.Register(&fakeScanner{name: "python", ecosystem: model.EcoPython})
	reg.Register(&fakeScanner{name: "xcode", ecosystem: model.EcoXcode})

	filtered := reg.ForEcosystems([]model.Ecosystem{model.EcoNode, model.EcoXcode})
	if len(filtered) != 2 {
		t.Fatalf("expected 2 scanners, got %d", len(filtered))
	}

	names := map[string]bool{}
	for _, s := range filtered {
		names[s.Name()] = true
	}
	if !names["node"] || !names["xcode"] {
		t.Errorf("expected node and xcode, got %v", names)
	}
}

func TestRegistryScanWith(t *testing.T) {
	reg := scanner.NewRegistry()
	nodeScanner := &fakeScanner{
		name:      "node",
		ecosystem: model.EcoNode,
		results:   []model.ScanResult{{Path: "/a", Size: 100}},
	}
	pythonScanner := &fakeScanner{
		name:      "python",
		ecosystem: model.EcoPython,
		results:   []model.ScanResult{{Path: "/b", Size: 200}},
	}
	reg.Register(nodeScanner)
	reg.Register(pythonScanner)

	results, err := reg.ScanWith(context.Background(), "/", []scanner.Scanner{nodeScanner})
	if err != nil {
		t.Fatalf("ScanWith error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestRegistryEmpty(t *testing.T) {
	reg := scanner.NewRegistry()
	results, err := reg.ScanAll(context.Background(), "/")
	if err != nil {
		t.Fatalf("ScanAll error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestModTime(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("hello"), 0o644)

	mt := scanner.ModTime(f)
	if mt.IsZero() {
		t.Error("expected non-zero ModTime")
	}
}

func TestModTimeNonExistent(t *testing.T) {
	mt := scanner.ModTime("/nonexistent/file")
	if !mt.IsZero() {
		t.Error("expected zero ModTime for nonexistent file")
	}
}

func TestRegistryScanWith_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reg := scanner.DefaultRegistry()
	_, err := reg.ScanWith(ctx, t.TempDir(), reg.ForEcosystems([]model.Ecosystem{model.EcoNode}))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("ScanWith(cancelled) error = %v, want context.Canceled", err)
	}
}
