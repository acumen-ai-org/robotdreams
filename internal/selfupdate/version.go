package selfupdate

import (
	"fmt"
	"strconv"
	"strings"
)

func Compare(a, b string) (int, error) {
	pa, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseVersion(b)
	if err != nil {
		return 0, err
	}

	for i := 0; i < 3; i++ {
		if pa.num[i] != pb.num[i] {
			if pa.num[i] < pb.num[i] {
				return -1, nil
			}
			return 1, nil
		}
	}
	return comparePrerelease(pa.pre, pb.pre), nil
}

type parsedVersion struct {
	num [3]int
	pre string
}

func parseVersion(v string) (parsedVersion, error) {
	orig := v
	v = TrimVersionPrefix(strings.TrimSpace(v))
	if v == "" {
		return parsedVersion{}, fmt.Errorf("selfupdate: empty version")
	}

	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var pre string
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre, v = v[i+1:], v[:i]
	}

	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return parsedVersion{}, fmt.Errorf("selfupdate: %q is not a semver version (want major.minor.patch)", orig)
	}

	var out parsedVersion
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return parsedVersion{}, fmt.Errorf("selfupdate: %q is not a semver version", orig)
		}
		out.num[i] = n
	}
	out.pre = pre
	return out, nil
}

func comparePrerelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}

	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] == bs[i] {
			continue
		}
		an, aErr := strconv.Atoi(as[i])
		bn, bErr := strconv.Atoi(bs[i])
		switch {
		case aErr == nil && bErr == nil:
			if an < bn {
				return -1
			}
			return 1
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		case as[i] < bs[i]:
			return -1
		default:
			return 1
		}
	}

	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}
