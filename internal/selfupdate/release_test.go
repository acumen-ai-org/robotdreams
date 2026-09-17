package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTargetFor(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		goos        string
		goarch      string
		wantArchive string
		wantBinary  string
		wantErr     bool
	}{
		{"linux amd64", "0.4.2", "linux", "amd64", "dream_0.4.2_linux_amd64.tar.gz", "dream", false},
		{"linux arm64", "0.4.2", "linux", "arm64", "dream_0.4.2_linux_arm64.tar.gz", "dream", false},
		{"darwin amd64", "0.4.2", "darwin", "amd64", "dream_0.4.2_darwin_amd64.tar.gz", "dream", false},
		{"darwin arm64", "0.4.2", "darwin", "arm64", "dream_0.4.2_darwin_arm64.tar.gz", "dream", false},
		{"windows amd64 is a zip", "0.4.2", "windows", "amd64", "dream_0.4.2_windows_amd64.zip", "dream.exe", false},
		{"windows arm64 is a zip", "0.4.2", "windows", "arm64", "dream_0.4.2_windows_arm64.zip", "dream.exe", false},
		{"leading v is trimmed", "v0.4.2", "linux", "amd64", "dream_0.4.2_linux_amd64.tar.gz", "dream", false},

		{"freebsd unsupported", "0.4.2", "freebsd", "amd64", "", "", true},
		{"386 unsupported", "0.4.2", "linux", "386", "", "", true},
		{"nonsense platform", "0.4.2", "sunos", "mips", "", "", true},
		{"empty version", "", "linux", "amd64", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TargetFor(tc.version, tc.goos, tc.goarch)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("TargetFor(%q,%q,%q) = %+v, want an error", tc.version, tc.goos, tc.goarch, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("TargetFor: %v", err)
			}
			if got.ArchiveName != tc.wantArchive {
				t.Fatalf("archive = %q, want %q", got.ArchiveName, tc.wantArchive)
			}
			if got.BinaryName != tc.wantBinary {
				t.Fatalf("binary = %q, want %q", got.BinaryName, tc.wantBinary)
			}
		})
	}
}

func TestTargetForUnsupportedIsTyped(t *testing.T) {
	if _, err := TargetFor("0.4.2", "plan9", "amd64"); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("err = %v, want ErrUnsupportedPlatform", err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the module root from the test's working directory")
		}
		dir = parent
	}
}

func TestGoreleaserContractUnchanged(t *testing.T) {
	path := filepath.Join(repoRoot(t), ".goreleaser.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var cfg struct {
		ProjectName string `yaml:"project_name"`
		Builds      []struct {
			Binary string   `yaml:"binary"`
			GOOS   []string `yaml:"goos"`
			GOARCH []string `yaml:"goarch"`
		} `yaml:"builds"`
		Archives []struct {
			Formats         []string `yaml:"formats"`
			NameTemplate    string   `yaml:"name_template"`
			FormatOverrides []struct {
				GOOS    string   `yaml:"goos"`
				Formats []string `yaml:"formats"`
			} `yaml:"format_overrides"`
		} `yaml:"archives"`
		Checksum struct {
			NameTemplate string `yaml:"name_template"`
		} `yaml:"checksum"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	const drift = "\n\nThe archive naming contract is defined in .goreleaser.yaml and mirrored in" +
		"\ninternal/selfupdate/release.go and npm/scripts/install.js. Update all three."

	if cfg.ProjectName != projectName {
		t.Errorf("project_name = %q, but release.go assumes %q%s", cfg.ProjectName, projectName, drift)
	}
	if cfg.Checksum.NameTemplate != checksumsAsset {
		t.Errorf("checksum name_template = %q, but release.go assumes %q%s",
			cfg.Checksum.NameTemplate, checksumsAsset, drift)
	}

	if len(cfg.Archives) != 1 {
		t.Fatalf("expected exactly one archive definition, got %d%s", len(cfg.Archives), drift)
	}
	ar := cfg.Archives[0]
	const wantTemplate = "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
	if ar.NameTemplate != wantTemplate {
		t.Errorf("archive name_template = %q, want %q%s", ar.NameTemplate, wantTemplate, drift)
	}
	if len(ar.Formats) != 1 || ar.Formats[0] != "tar.gz" {
		t.Errorf("archive formats = %v, want [tar.gz]%s", ar.Formats, drift)
	}
	foundWindowsZip := false
	for _, o := range ar.FormatOverrides {
		if o.GOOS == "windows" && len(o.Formats) == 1 && o.Formats[0] == "zip" {
			foundWindowsZip = true
		}
	}
	if !foundWindowsZip {
		t.Errorf("no windows->zip format override; release.go assumes windows assets are .zip%s", drift)
	}

	if len(cfg.Builds) != 1 {
		t.Fatalf("expected exactly one build definition, got %d%s", len(cfg.Builds), drift)
	}
	b := cfg.Builds[0]
	if b.Binary != binaryName {
		t.Errorf("build binary = %q, but release.go assumes %q%s", b.Binary, binaryName, drift)
	}
	assertSameSet(t, "goos", b.GOOS, supportedGOOS, drift)
	assertSameSet(t, "goarch", b.GOARCH, supportedGOARCH, drift)
}

func assertSameSet(t *testing.T, label string, got, want []string, drift string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, but release.go supports %v%s", label, got, want, drift)
		return
	}
	inWant := map[string]bool{}
	for _, w := range want {
		inWant[w] = true
	}
	for _, g := range got {
		if !inWant[g] {
			t.Errorf("%s includes %q, which release.go does not support (supports %v)%s", label, g, want, drift)
		}
	}
}
