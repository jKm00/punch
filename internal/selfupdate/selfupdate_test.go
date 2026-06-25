package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"v1.0.0", "v1.0.1", -1},
		{"v1.1.0", "v1.0.9", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0.0", "v1.0.0-rc1", 0}, // prerelease suffix ignored for core compare
		{"v1.2", "v1.2.0", 0},       // missing patch treated as 0
		{"v1.10.0", "v1.9.0", 1},    // numeric, not lexical
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("v1.0.0", "v1.0.1") {
		t.Error("v1.0.1 should be newer than v1.0.0")
	}
	if IsNewer("v1.0.1", "v1.0.0") {
		t.Error("v1.0.0 should not be newer than v1.0.1")
	}
	if IsNewer("v1.0.0", "v1.0.0") {
		t.Error("equal versions are not newer")
	}
	// dev builds are never considered outdated.
	if IsNewer("dev", "v9.9.9") {
		t.Error("dev build should never be flagged as outdated")
	}
	if IsNewer("", "v1.0.0") {
		t.Error("empty version should never be flagged as outdated")
	}
}

func TestAssetNameFor(t *testing.T) {
	got := AssetNameFor("v1.2.3", "darwin", "arm64")
	want := "punch_1.2.3_darwin_arm64.tar.gz"
	if got != want {
		t.Errorf("AssetNameFor = %q, want %q", got, want)
	}
	// Leading v is stripped even if missing.
	if AssetNameFor("1.2.3", "linux", "amd64") != "punch_1.2.3_linux_amd64.tar.gz" {
		t.Errorf("unexpected: %q", AssetNameFor("1.2.3", "linux", "amd64"))
	}
}

func TestChecksumFor(t *testing.T) {
	sums := "" +
		"abc123  punch_1.2.3_darwin_arm64.tar.gz\n" +
		"def456 *punch_1.2.3_linux_amd64.tar.gz\n" +
		"\n" +
		"ghi789  ./punch_1.2.3_linux_arm64.tar.gz\n"

	if v, ok := ChecksumFor(sums, "punch_1.2.3_darwin_arm64.tar.gz"); !ok || v != "abc123" {
		t.Errorf("darwin arm64: got %q ok=%v", v, ok)
	}
	// Tolerates "*" binary-mode prefix.
	if v, ok := ChecksumFor(sums, "punch_1.2.3_linux_amd64.tar.gz"); !ok || v != "def456" {
		t.Errorf("linux amd64: got %q ok=%v", v, ok)
	}
	// Tolerates "./" prefix.
	if v, ok := ChecksumFor(sums, "punch_1.2.3_linux_arm64.tar.gz"); !ok || v != "ghi789" {
		t.Errorf("linux arm64: got %q ok=%v", v, ok)
	}
	if _, ok := ChecksumFor(sums, "missing.tar.gz"); ok {
		t.Error("missing file should not be found")
	}
}

func TestExtractBinary(t *testing.T) {
	want := []byte("fake-binary-contents")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// Add a decoy file and the real binary.
	writeTar(t, tw, "README.txt", []byte("ignore me"))
	writeTar(t, tw, "punch", want)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractBinary(buf.Bytes(), "punch")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted %q, want %q", got, want)
	}

	if _, err := extractBinary(buf.Bytes(), "nonexistent"); err == nil {
		t.Error("expected error for missing binary in archive")
	}
}

func writeTar(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
}
