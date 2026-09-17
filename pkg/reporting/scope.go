package reporting

import "strings"

// SplitScope splits a scope path into its non-empty, trimmed segments.
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

// ScopeDepth returns the number of segments in a scope path; the empty path has depth 0.
func ScopeDepth(path string) int {
	return len(SplitScope(path))
}

// LevelName returns the advisory level name for a scope depth: root, universe, world, realm or site.
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

// ScopeWithin reports whether child is at or below parent.
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

// ParentScope returns the scope path one level up, or "" for paths of depth 0 or 1.
func ParentScope(path string) string {
	segs := SplitScope(path)
	if len(segs) <= 1 {
		return ""
	}
	return strings.Join(segs[:len(segs)-1], "/")
}
