package selfupdate

import "testing"

func TestChecksumFor(t *testing.T) {
	const sum = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	tests := []struct {
		name    string
		text    string
		asset   string
		want    string
		wantErr bool
	}{
		{
			name:  "two-space separated",
			text:  sum + "  dream_0.4.2_linux_amd64.tar.gz\n",
			asset: "dream_0.4.2_linux_amd64.tar.gz",
			want:  sum,
		},
		{
			name:  "binary mode asterisk",
			text:  sum + " *dream_0.4.2_linux_amd64.tar.gz\n",
			asset: "dream_0.4.2_linux_amd64.tar.gz",
			want:  sum,
		},
		{
			name:  "CRLF line endings",
			text:  sum + "  dream_0.4.2_linux_amd64.tar.gz\r\n",
			asset: "dream_0.4.2_linux_amd64.tar.gz",
			want:  sum,
		},
		{
			name:  "uppercase hex is normalized",
			text:  "9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08  a.tar.gz\n",
			asset: "a.tar.gz",
			want:  sum,
		},
		{
			name: "picks the right line among many",
			text: "1111111111111111111111111111111111111111111111111111111111111111  other.tar.gz\n" +
				sum + "  dream_0.4.2_linux_amd64.tar.gz\n" +
				"2222222222222222222222222222222222222222222222222222222222222222  another.zip\n",
			asset: "dream_0.4.2_linux_amd64.tar.gz",
			want:  sum,
		},
		{
			name:    "asset absent",
			text:    sum + "  other.tar.gz\n",
			asset:   "dream_0.4.2_linux_amd64.tar.gz",
			wantErr: true,
		},
		{
			name:    "truncated hex is rejected",
			text:    "abc123  dream_0.4.2_linux_amd64.tar.gz\n",
			asset:   "dream_0.4.2_linux_amd64.tar.gz",
			wantErr: true,
		},
		{
			name:    "empty file",
			text:    "",
			asset:   "dream_0.4.2_linux_amd64.tar.gz",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ChecksumFor(tc.text, tc.asset)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ChecksumFor = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ChecksumFor: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	const a = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	if err := VerifySHA256(a, a); err != nil {
		t.Fatalf("identical digests: %v", err)
	}
	if err := VerifySHA256(a, "9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08"); err != nil {
		t.Fatalf("case-insensitive compare failed: %v", err)
	}
	if err := VerifySHA256(a, "1111111111111111111111111111111111111111111111111111111111111111"); err == nil {
		t.Fatal("mismatched digests compared equal")
	}
}
