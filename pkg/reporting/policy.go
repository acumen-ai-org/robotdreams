package reporting

import (
	"fmt"
	"strconv"
	"strings"
)

// Policy is a parsed aggregation policy. N carries the parameter for the
// parameterized policies sample(n) and top(n); it is 0 for every other
// policy.
type Policy struct {
	Name string
	N    int
}

// bare policies take no parameter.
var barePolicies = []string{
	"worst", "sum", "avg", "min", "max", "p50", "p95",
	"latest", "count", "merge", "synthesize",
}

// ParsePolicy parses an aggregation policy string: either a bare name from
// the policy vocabulary (worst, sum, avg, min, max, p50, p95, latest,
// count, merge, synthesize) or a parameterized form sample(n) / top(n)
// with a positive integer n.
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
