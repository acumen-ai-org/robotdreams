package reporting

import (
	"fmt"
	"strconv"
	"strings"
)

// Policy is a parsed aggregation policy.
type Policy struct {
	Name string
	N    int
}

const (
	defaultStatusPolicy   = "worst"
	defaultHeadlinePolicy = "latest"
	defaultKPIPolicy      = "latest"
	defaultItemsPolicy    = "merge"
)

var barePolicies = []string{
	"worst", "sum", "avg", "min", "max", "p50", "p95",
	"latest", "count", "merge", "synthesize",
}

// ParsePolicy parses a bare policy name or the parameterized forms sample(n) and top(n).
func ParsePolicy(s string) (Policy, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Policy{}, fmt.Errorf("empty aggregation policy")
	}
	if i := strings.IndexByte(s, '('); i >= 0 {
		name := s[:i]
		if !strings.HasSuffix(s, ")") {
			return Policy{}, fmt.Errorf("malformed policy %q: missing closing parenthesis", s)
		}
		if name != "sample" && name != "top" {
			return Policy{}, fmt.Errorf("policy %q does not take a parameter", name)
		}
		arg := strings.TrimSpace(s[i+1 : len(s)-1])
		n, err := strconv.Atoi(arg)
		if err != nil || n <= 0 {
			return Policy{}, fmt.Errorf("policy %q: parameter must be a positive integer", s)
		}
		return Policy{Name: name, N: n}, nil
	}
	if contains(barePolicies, s) {
		return Policy{Name: s}, nil
	}
	if s == "sample" || s == "top" {
		return Policy{}, fmt.Errorf("policy %q requires a parameter, e.g. %s(10)", s, s)
	}
	return Policy{}, fmt.Errorf("unknown aggregation policy %q", s)
}
