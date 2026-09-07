package version

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
)

func TestW6BRejectsTagPrefixedArchiveName(t *testing.T) {
	suffix, err := assetSuffix()
	if err != nil {
		t.Skipf("unsupported test platform: %v", err)
	}
	name := "ainovel-cli_v1.2.3" + suffix
	rel := release{
		TagName: "v1.2.3",
		Assets: []releaseAsset{{
			Name:               name,
			BrowserDownloadURL: "https://github.com/" + ProductRepository + "/releases/download/v1.2.3/" + name,
		}},
	}
	if _, err := selectAsset(&rel, "ainovel-cli"); err == nil {
		t.Fatal("expected tag-prefixed archive name to be rejected")
	}
}

func TestW6BRejectsOffForkArchiveURL(t *testing.T) {
	suffix, err := assetSuffix()
	if err != nil {
		t.Skipf("unsupported test platform: %v", err)
	}
	name := "ainovel-cli_1.2.3" + suffix
	rel := release{
		TagName: "v1.2.3",
		Assets: []releaseAsset{{
			Name:               name,
			BrowserDownloadURL: "https://github.com/kentjuno/ainovel-cli/releases/download/v1.2.3/" + name,
		}},
	}
	if _, err := selectAsset(&rel, "ainovel-cli"); err == nil {
		t.Fatal("expected off-fork archive URL to be rejected")
	}
}

func TestW6BRejectsOffForkChecksumURL(t *testing.T) {
	name := "ainovel-cli_checksums.txt"
	rel := release{
		TagName: "v1.2.3",
		Assets: []releaseAsset{{
			Name:               name,
			BrowserDownloadURL: "https://github.com/kentjuno/ainovel-cli/releases/download/v1.2.3/" + name,
		}},
	}
	if _, err := selectChecksumAsset(&rel, "ainovel-cli"); err == nil {
		t.Fatal("expected off-fork checksum URL to be rejected")
	}
}

func TestW6BRejectsDuplicateChecksumEntries(t *testing.T) {
	root := t.TempDir()
	archivePath := root + "/asset.tar.gz"
	checksumPath := root + "/checksums.txt"
	archive := []byte("archive")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive)
	line := fmt.Sprintf("%x  asset.tar.gz\n", sum)
	if err := os.WriteFile(checksumPath, []byte(line+line), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(archivePath, checksumPath, "asset.tar.gz"); err == nil {
		t.Fatal("expected duplicate checksum entries to fail closed")
	}
}

func TestW6BRejectsNestedArchiveBinary(t *testing.T) {
	root := t.TempDir()
	archivePath := root + "/asset.tar.gz"
	archive := makeW6BTarGz(t, "nested/ainovel-cli", []byte("binary"))
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := extractBinary(archivePath, t.TempDir(), "ainovel-cli"); err == nil {
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
