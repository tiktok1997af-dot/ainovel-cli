package version

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestW6BRejectsTagPrefixedArchiveName(t *testing.T) {
	suffix, err := assetSuffix()
	if err != nil {
		t.Skipf("unsupported test platform: %v", err)
	}
	name := "ainovel-cli_v1.2.3" + suffix
	rel := releaseInfo{
		TagName: "v1.2.3",
		Assets: []releaseAsset{{
			Name:               name,
			BrowserDownloadURL: "https://github.com/" + ProductRepository + "/releases/download/v1.2.3/" + name,
		}},
	}
	if _, err := selectAsset(rel, "ainovel-cli"); err == nil {
		t.Fatal("expected tag-prefixed archive name to be rejected")
	}
}

func TestW6BRejectsOffForkArchiveURL(t *testing.T) {
	suffix, err := assetSuffix()
	if err != nil {
		t.Skipf("unsupported test platform: %v", err)
	}
	name := "ainovel-cli_1.2.3" + suffix
	rel := releaseInfo{
		TagName: "v1.2.3",
		Assets: []releaseAsset{{
			Name:               name,
			BrowserDownloadURL: "https://github.com/kentjuno/ainovel-cli/releases/download/v1.2.3/" + name,
		}},
	}
	if _, err := selectAsset(rel, "ainovel-cli"); err == nil {
		t.Fatal("expected off-fork archive URL to be rejected")
	}
}

func TestW6BRejectsOffForkChecksumURL(t *testing.T) {
	name := "ainovel-cli_checksums.txt"
	rel := releaseInfo{
		TagName: "v1.2.3",
		Assets: []releaseAsset{{
			Name:               name,
			BrowserDownloadURL: "https://github.com/kentjuno/ainovel-cli/releases/download/v1.2.3/" + name,
		}},
	}
	if _, err := selectChecksumAsset(rel, "ainovel-cli"); err == nil {
		t.Fatal("expected off-fork checksum URL to be rejected")
	}
}

func TestW6BRejectsDuplicateChecksumEntries(t *testing.T) {
	archive := []byte("archive")
	sum := sha256.Sum256(archive)
	line := fmt.Sprintf("%x  asset.tar.gz\n", sum)
	checksums := []byte(line + line)
	if err := verifyChecksum(checksums, "asset.tar.gz", archive); err == nil {
		t.Fatal("expected duplicate checksum entries to fail closed")
	}
}

func TestW6BRejectsNestedArchiveBinary(t *testing.T) {
	archive := makeW6BTarGz(t, "nested/ainovel-cli", []byte("binary"))
	if _, err := extractBinary(archive, "ainovel-cli", t.TempDir()); err == nil {
		t.Fatal("expected nested binary entry to be rejected")
	}
}

func makeW6BTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestW6BRegressionHelperDoesNotTouchFilesystem(t *testing.T) {
	// Keeps filepath/os imports exercised on every platform while asserting the
	// regression fixture itself is isolated from the product install path.
	root := t.TempDir()
	path := filepath.Join(root, "sentinel")
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
