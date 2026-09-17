package reporting

import "testing"

func TestParsePolicy(t *testing.T) {
	valid := []struct {
		in   string
		want Policy
	}{
		{"worst", Policy{Name: "worst"}},
		{"sum", Policy{Name: "sum"}},
		{"avg", Policy{Name: "avg"}},
		{"min", Policy{Name: "min"}},
		{"max", Policy{Name: "max"}},
		{"p50", Policy{Name: "p50"}},
		{"p95", Policy{Name: "p95"}},
		{"latest", Policy{Name: "latest"}},
		{"count", Policy{Name: "count"}},
		{"merge", Policy{Name: "merge"}},
		{"synthesize", Policy{Name: "synthesize"}},
		{"sample(50)", Policy{Name: "sample", N: 50}},
		{"top(3)", Policy{Name: "top", N: 3}},
		{"sample( 5 )", Policy{Name: "sample", N: 5}},
		{"  worst  ", Policy{Name: "worst"}},
	}
	for _, tt := range valid {
		got, err := ParsePolicy(tt.in)
		if err != nil {
			t.Errorf("ParsePolicy(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParsePolicy(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}

	invalid := []string{
		"",
		"bogus",
		"sample",
		"top",
		"sample()",
		"sample(0)",
		"sample(-1)",
		"sample(x)",
		"top(",
		"top(2",
		"latest(2)",
		"worst(1)",
	}
	for _, in := range invalid {
		if got, err := ParsePolicy(in); err == nil {
			t.Errorf("ParsePolicy(%q) = %+v, want error", in, got)
		}
	}
}
