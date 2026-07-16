package scanner

import (
	"path/filepath"
	"testing"
)

func TestIsUnderRoot(t *testing.T) {
	root := filepath.FromSlash("/home/user")

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"same as root", root, true},
		{"direct child", filepath.FromSlash("/home/user/.npm"), true},
		{"nested descendant", filepath.FromSlash("/home/user/.cache/pip"), true},
		{"sibling outside", filepath.FromSlash("/home/other"), false},
		{"parent outside", filepath.FromSlash("/home"), false},
		{"exact parent ref", filepath.FromSlash("/private/var/folders"), false},
		// Regression: a child dir whose name literally begins with ".."
		// must not be misread as "outside root" just because Rel starts with "..".
		{"dotdot-prefixed child name", filepath.FromSlash("/home/user/..iCloud"), true},
		{"dotdot-prefixed nested", filepath.FromSlash("/home/user/..iCloud/data"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUnderRoot(tc.path, root); got != tc.want {
				t.Errorf("isUnderRoot(%q, %q) = %v, want %v", tc.path, root, got, tc.want)
			}
		})
	}
}
