package cli

import "testing"

func TestParseMinSize(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"1024", 1024, false},
		{"1KB", 1000, false},
		{"1KiB", 1024, false},
		{"1MB", 1_000_000, false},
		{"500KB", 500_000, false},
		{"2.5GB", 2_500_000_000, false},
		{"abc", 0, true},
		{"-1MB", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseMinSize(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}
