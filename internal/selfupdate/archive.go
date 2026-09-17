package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxBinaryBytes = 256 << 20

func ExtractBinary(archivePath, binaryName, destPath string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractFromZip(archivePath, binaryName, destPath)
	}
	return extractFromTarGz(archivePath, binaryName, destPath)
}

func safeEntryName(name, want string) bool {
	if name != want {
		return false
	}
	if strings.ContainsAny(name, `/\`) || name == ".." {
		return false
	}
	return filepath.Base(name) == name
}

func extractFromTarGz(archivePath, binaryName, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("selfupdate: open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("selfupdate: read gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("selfupdate: read tar: %w", err)
		}
		if !safeEntryName(hdr.Name, binaryName) {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("selfupdate: archive entry %q is not a regular file (type %q)", hdr.Name, string(hdr.Typeflag))
		}
		return writeBinary(destPath, tr)
	}
	return fmt.Errorf("selfupdate: archive %s contains no %q at its root", filepath.Base(archivePath), binaryName)
}

func extractFromZip(archivePath, binaryName, destPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("selfupdate: open zip: %w", err)
	}
	defer zr.Close()

	for _, entry := range zr.File {
		if !safeEntryName(entry.Name, binaryName) {
			continue
		}
		if !entry.Mode().IsRegular() {
			return fmt.Errorf("selfupdate: archive entry %q is not a regular file", entry.Name)
		}
		rc, err := entry.Open()
		if err != nil {
			return fmt.Errorf("selfupdate: open zip entry: %w", err)
		}
		defer rc.Close()
		return writeBinary(destPath, rc)
	}
	return fmt.Errorf("selfupdate: archive %s contains no %q at its root", filepath.Base(archivePath), binaryName)
}

func writeBinary(destPath string, r io.Reader) error {
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755) //nolint:gosec // G302: it is the executable being installed
	if err != nil {
		return fmt.Errorf("selfupdate: create %s: %w", destPath, err)
	}
	defer out.Close()

	n, err := io.Copy(out, io.LimitReader(r, maxBinaryBytes+1))
	if err != nil {
		return fmt.Errorf("selfupdate: write %s: %w", destPath, err)
	}
	if n > maxBinaryBytes {
		return fmt.Errorf("selfupdate: extracted binary exceeds %d bytes", maxBinaryBytes)
	}

	if err := out.Chmod(0o755); err != nil {
		return fmt.Errorf("selfupdate: chmod %s: %w", destPath, err)
	}
	return out.Sync()
}
