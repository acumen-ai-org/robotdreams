package selfupdate

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.4.2", "0.4.2", 0},
		{"0.4.1", "0.4.2", -1},
		{"0.4.2", "0.4.1", 1},
		{"0.5.0", "0.4.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"v0.4.2", "0.4.2", 0},
		{"0.4.2", "v0.4.2", 0},

		{"1.0.0+build.1", "1.0.0+build.2", 0},
		{"1.0.0+build.1", "1.0.0", 0},

		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-rc.1", "1.0.0-rc.2", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha", 1},
		{"1.0.0-1", "1.0.0-alpha", -1},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			got, err := Compare(tc.a, tc.b)
			if err != nil {
				t.Fatalf("Compare(%q,%q): %v", tc.a, tc.b, err)
			}
			if got != tc.want {
				t.Fatalf("Compare(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCompareRejectsNonSemver(t *testing.T) {
	for _, v := range []string{"", "2026-09-01-g1a2b3c", "sha256:9f86d0", "1.2.3.4", "abc", "1", "1.2"} {
		if _, err := Compare(v, "1.0.0"); err == nil {
			t.Fatalf("Compare(%q, ...) succeeded, want an error", v)
		}
	}
}

func TestTrimVersionPrefix(t *testing.T) {
	tests := []struct{ in, want string }{
		{"v0.4.2", "0.4.2"},
		{"0.4.2", "0.4.2"},
		{"v", "v"},
		{"", ""},
		{"version", "ersion"},
	}
	for _, tc := range tests {
		if got := TrimVersionPrefix(tc.in); got != tc.want {
			t.Fatalf("TrimVersionPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
