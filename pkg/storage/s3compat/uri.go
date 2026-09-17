package s3compat

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const defaultRegion = "us-east-1"

type config struct {
	bucket string

	prefix string

	endpoint string

	region string

	usePathStyle bool

	accessKey string
	secretKey string
}

func parseURI(u *url.URL) (config, error) {
	if u == nil {
		return config{}, fmt.Errorf("s3compat: nil URI")
	}
	if u.Scheme != "s3" {
		return config{}, fmt.Errorf("s3compat: unsupported scheme %q, want \"s3\"", u.Scheme)
	}
	bucket := u.Host
	if bucket == "" {
		return config{}, fmt.Errorf("s3compat: URI %q has no bucket (expected s3://bucket/prefix)", u.String())
	}

	q := u.Query()
	cfg := config{
		bucket:    bucket,
		prefix:    strings.Trim(u.Path, "/"),
		endpoint:  q.Get("endpoint"),
		region:    q.Get("region"),
		accessKey: q.Get("access-key"),
		secretKey: q.Get("secret-key"),
	}

	if v := q.Get("use-path-style"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return config{}, fmt.Errorf("s3compat: invalid use-path-style value %q: %w", v, err)
		}
		cfg.usePathStyle = b
	}

	if cfg.region == "" {
		cfg.region = defaultRegion
	}

	return cfg, nil
}
