package reporting

import (
	"strings"
	"testing"
)

func TestSplitScope(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"", nil},
		{"/", nil},
		{"acme", []string{"acme"}},
		{"acme/music/platform/deploy-squad", []string{"acme", "music", "platform", "deploy-squad"}},
		{"/acme//music/", []string{"acme", "music"}},
		{" acme / music ", []string{"acme", "music"}},
	}
	for _, tt := range tests {
		got := SplitScope(tt.path)
		if strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("SplitScope(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestScopeDepthAndLevelName(t *testing.T) {
	depths := []struct {
		path string
		want int
	}{
		{"", 0},
		{"acme", 1},
		{"acme/music", 2},
		{"acme//music/", 2},
		{"a/b/c/d/e", 5},
	}
	for _, tt := range depths {
		if got := ScopeDepth(tt.path); got != tt.want {
			t.Errorf("ScopeDepth(%q) = %d, want %d", tt.path, got, tt.want)
		}
	}

	levels := []struct {
		depth int
		want  string
	}{
		{-1, "root"},
		{0, "root"},
		{1, "universe"},
		{2, "world"},
		{3, "realm"},
		{4, "site"},
		{5, "site"},
		{9, "site"},
	}
	for _, tt := range levels {
		if got := LevelName(tt.depth); got != tt.want {
			t.Errorf("LevelName(%d) = %q, want %q", tt.depth, got, tt.want)
		}
	}
}

func TestScopeWithin(t *testing.T) {
	tests := []struct {
		parent, child string
		want          bool
	}{
		{"", "", true},
		{"", "acme/music", true},
		{"acme", "acme", true},
		{"acme", "acme/music", true},
		{"acme/music", "acme/music/platform/deploy-squad", true},
		{"acme/music", "acme", false},
		{"acme", "music/acme", false},
		{"acme/music", "acme/movies", false},
		{"acme", "acmecorp", false},
		{"acme/", "acme//music", true},
	}
	for _, tt := range tests {
		if got := ScopeWithin(tt.parent, tt.child); got != tt.want {
			t.Errorf("ScopeWithin(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
		}
	}
}

func TestParentScope(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"", ""},
		{"acme", ""},
		{"acme/music", "acme"},
		{"acme/music/platform", "acme/music"},
		{"/acme//music/", "acme"},
	}
	for _, tt := range tests {
		if got := ParentScope(tt.path); got != tt.want {
			t.Errorf("ParentScope(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
