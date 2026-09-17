package s3compat

import (
	"net/url"
	"testing"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u
}

func TestParseURI(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want config
	}{
		{
			name: "bucket only",
			uri:  "s3://my-bucket",
			want: config{bucket: "my-bucket", prefix: "", region: defaultRegion},
		},
		{
			name: "bucket and prefix",
			uri:  "s3://my-bucket/some/prefix",
			want: config{bucket: "my-bucket", prefix: "some/prefix", region: defaultRegion},
		},
		{
			name: "trailing slash prefix trimmed",
			uri:  "s3://my-bucket/some/prefix/",
			want: config{bucket: "my-bucket", prefix: "some/prefix", region: defaultRegion},
		},
		{
			name: "endpoint override and path style",
			uri:  "s3://my-bucket/prefix?endpoint=http://localhost:9000&region=us-east-1&use-path-style=true",
			want: config{
				bucket:       "my-bucket",
				prefix:       "prefix",
				endpoint:     "http://localhost:9000",
				region:       "us-east-1",
				usePathStyle: true,
			},
		},
		{
			name: "explicit region overrides default",
			uri:  "s3://my-bucket?region=eu-west-1",
			want: config{bucket: "my-bucket", region: "eu-west-1"},
		},
		{
			name: "dev credentials fallback",
			uri:  "s3://my-bucket?access-key=minioadmin&secret-key=minioadmin",
			want: config{bucket: "my-bucket", region: defaultRegion, accessKey: "minioadmin", secretKey: "minioadmin"},
		},
		{
			name: "use-path-style false explicit",
			uri:  "s3://my-bucket?use-path-style=false",
			want: config{bucket: "my-bucket", region: defaultRegion, usePathStyle: false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseURI(mustParseURL(t, tc.uri))
			if err != nil {
				t.Fatalf("parseURI(%q): %v", tc.uri, err)
			}
			if got != tc.want {
				t.Fatalf("parseURI(%q) = %+v, want %+v", tc.uri, got, tc.want)
			}
		})
	}
}

func TestParseURIErrors(t *testing.T) {
	cases := []struct {
		name string
		uri  string
	}{
		{name: "wrong scheme", uri: "file:///tmp/x"},
		{name: "no bucket", uri: "s3:///prefix"},
		{name: "invalid use-path-style", uri: "s3://bucket?use-path-style=maybe"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseURI(mustParseURL(t, tc.uri)); err == nil {
				t.Fatalf("parseURI(%q): expected error, got nil", tc.uri)
			}
		})
	}
}

func TestParseURINilURL(t *testing.T) {
	if _, err := parseURI(nil); err == nil {
		t.Fatalf("parseURI(nil): expected error, got nil")
	}
}
