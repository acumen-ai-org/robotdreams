package reporting

import "strings"

// Scope paths are slash-separated labels, not enforced containers, e.g.
// "acme/music/platform/deploy-squad". Depth is what the code operates on;
// the level names are advisory (see contracts/aggregations.yaml).

// SplitScope splits a scope path into its segments, trimming whitespace
// and dropping empty segments, so "/acme//music/" yields [acme music].
func SplitScope(path string) []string {
	var segs []string
	for _, s := range strings.Split(path, "/") {
		s = strings.TrimSpace(s)
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs
}

// ScopeDepth returns the number of segments in a scope path; the empty
// path has depth 0.
func ScopeDepth(path string) int {
	return len(SplitScope(path))
}

// LevelName returns the advisory level name for a scope path depth:
// 1..4 map to universe, world, realm, site; anything deeper is still
// "site"; depth 0 (or less) is "root".
func LevelName(depth int) string {
	switch {
	case depth <= 0:
		return "root"
	case depth <= len(ScopeLevels):
		return ScopeLevels[depth-1]
	default:
		return ScopeLevels[len(ScopeLevels)-1]
	}
}

// ScopeWithin reports whether child is at or below parent: parent's
// segments must be a prefix of child's. The empty parent contains every
// scope, and every scope is within itself.
func ScopeWithin(parent, child string) bool {
	p := SplitScope(parent)
	c := SplitScope(child)
	if len(p) > len(c) {
		return false
	}
	for i := range p {
		if p[i] != c[i] {
			return false
		}
	}
	return true
}

// ParentScope returns the scope path one level up, or "" for paths of
// depth 0 or 1.
func ParentScope(path string) string {
	segs := SplitScope(path)
	if len(segs) <= 1 {
		return ""
	}
	return strings.Join(segs[:len(segs)-1], "/")
}
