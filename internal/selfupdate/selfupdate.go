// Package selfupdate handles version comparison, checking the GitHub
// Enterprise Releases API for newer versions, and replacing the running binary
// in place (`punch upgrade`).
//
// Repo coordinates are defined here as the single source of truth.
package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo coordinates on github.com.
const (
	// Owner and Repo identify the releases source.
	Owner = "jKm00"
	Repo  = "punch"

	// apiBase is the github.com REST API base URL.
	apiBase = "https://api.github.com"
)

// httpTimeout bounds every network call so the CLI never hangs.
const httpTimeout = 8 * time.Second

// AssetName returns the expected release asset name for the given version and
// the current OS/arch, e.g. "punch_1.2.3_darwin_arm64.tar.gz". version may be
// with or without a leading "v"; the asset uses the bare number.
func AssetName(version string) string {
	return AssetNameFor(version, runtime.GOOS, runtime.GOARCH)
}

// AssetNameFor is AssetName with explicit os/arch (for testing).
func AssetNameFor(version, goos, goarch string) string {
	v := strings.TrimPrefix(version, "v")
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", Repo, v, goos, goarch)
}

// ---- semver comparison ----

// CompareVersions compares two version strings of the form "vX.Y.Z" (or
// "X.Y.Z", optionally with a "-suffix" prerelease that is ignored for ordering
// of the core numbers). It returns -1 if a < b, 0 if equal, +1 if a > b.
// Non-numeric / unparseable components are treated as 0.
func CompareVersions(a, b string) int {
	an := parseSemver(a)
	bn := parseSemver(b)
	for i := 0; i < 3; i++ {
		if an[i] < bn[i] {
			return -1
		}
		if an[i] > bn[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	// Drop build/prerelease suffix.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	parts := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err == nil {
			out[i] = n
		}
	}
	return out
}

// IsNewer reports whether latest is strictly newer than current. A current
// version of "dev" (unstamped local build) is treated as never-outdated so dev
// builds are not nagged.
func IsNewer(current, latest string) bool {
	if current == "" || current == "dev" {
		return false
	}
	return CompareVersions(latest, current) > 0
}

// ---- releases API ----

// Release is the subset of the GitHub release payload we use.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is one uploaded release artifact.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"` // API URL (asset id), used with Accept: octet-stream
}

// token returns an auth token from the environment, if any. GHE often requires
// auth even for reads.
func token() string {
	for _, k := range []string{"PUNCH_GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func newClient() *http.Client {
	return &http.Client{Timeout: httpTimeout}
}

func authHeaders(req *http.Request) {
	if t := token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

// LatestRelease fetches the latest release for the repo. It returns the release
// or an error. Callers that want graceful degradation should ignore the error.
func LatestRelease() (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBase, Owner, Repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	authHeaders(req)

	resp, err := newClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("releases API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// ---- daily cached check ----

// checkState persists the last check time and the last seen latest tag.
type checkState struct {
	LastCheck time.Time `json:"last_check"`
	LatestTag string    `json:"latest_tag"`
}

func statePath() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "punch", "update-check.json"), nil
}

func loadState() checkState {
	var st checkState
	p, err := statePath()
	if err != nil {
		return st
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	return st
}

func saveState(st checkState) {
	p, err := statePath()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, b, 0o644)
}

// checkInterval is how often a network check is allowed.
const checkInterval = 24 * time.Hour

// disabled reports whether automatic checks are turned off via env.
func disabled() bool {
	v := os.Getenv("PUNCH_NO_UPDATE_CHECK")
	return v != "" && v != "0" && v != "false"
}

// PendingNotice returns a non-empty "new version" message if the cached state
// already knows of a newer version than current. This is cheap (no network)
// and is shown at the START of a command using whatever the last background
// check found.
func PendingNotice(current string) string {
	if disabled() {
		return ""
	}
	st := loadState()
	if st.LatestTag == "" || !IsNewer(current, st.LatestTag) {
		return ""
	}
	return fmt.Sprintf("A new version of punch is available (%s → %s). Run `punch upgrade` to update.",
		current, st.LatestTag)
}

// MaybeRefresh performs a network check at most once per checkInterval, updating
// the cached state. It is intended to run AFTER a command (in the background or
// just before exit) so the notice appears on the next invocation and never adds
// latency to the current one. Errors are swallowed (graceful degradation).
func MaybeRefresh(current string) {
	if disabled() || current == "" || current == "dev" {
		return
	}
	st := loadState()
	if time.Since(st.LastCheck) < checkInterval {
		return
	}
	st.LastCheck = time.Now()
	if rel, err := LatestRelease(); err == nil && rel.TagName != "" {
		st.LatestTag = rel.TagName
	}
	saveState(st)
}

// ---- upgrade (self-replace) ----

// UpgradeResult describes the outcome of an upgrade attempt.
type UpgradeResult struct {
	From string
	To   string
}

// ErrUpToDate indicates no upgrade was necessary.
var ErrUpToDate = errors.New("already up to date")

// Upgrade downloads the latest release asset for this OS/arch, verifies its
// SHA256 against the release's SHA256SUMS, and atomically replaces the running
// executable. current is the running version. progress, if non-nil, receives
// human-readable status lines.
func Upgrade(current string, progress func(string)) (*UpgradeResult, error) {
	log := func(s string) {
		if progress != nil {
			progress(s)
		}
	}
	if current == "dev" {
		return nil, errors.New("this is a dev build; install a released version to use upgrade")
	}

	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate current executable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	if err := guardWritable(exePath); err != nil {
		return nil, err
	}

	log("Checking latest release…")
	rel, err := LatestRelease()
	if err != nil {
		return nil, fmt.Errorf("check latest release: %w", err)
	}
	if !IsNewer(current, rel.TagName) {
		return nil, ErrUpToDate
	}

	assetName := AssetName(rel.TagName)
	asset := findAsset(rel, assetName)
	if asset == nil {
		return nil, fmt.Errorf("release %s has no asset %q for this platform", rel.TagName, assetName)
	}
	sums := findAsset(rel, "SHA256SUMS")
	if sums == nil {
		return nil, fmt.Errorf("release %s has no SHA256SUMS asset", rel.TagName)
	}

	log(fmt.Sprintf("Downloading %s…", assetName))
	archive, err := downloadAsset(asset)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}

	log("Verifying checksum…")
	sumsData, err := downloadAsset(sums)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	want, ok := ChecksumFor(string(sumsData), assetName)
	if !ok {
		return nil, fmt.Errorf("no checksum for %s in SHA256SUMS", assetName)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("checksum mismatch for %s", assetName)
	}

	log("Extracting…")
	binData, err := extractBinary(archive, Repo)
	if err != nil {
		return nil, fmt.Errorf("extract binary: %w", err)
	}

	log("Replacing binary…")
	if err := replaceExecutable(exePath, binData); err != nil {
		return nil, err
	}

	return &UpgradeResult{From: current, To: rel.TagName}, nil
}

// guardWritable refuses to upgrade binaries that look package-manager-managed
// or are not writable by the current user.
func guardWritable(exePath string) error {
	lower := strings.ToLower(exePath)
	for _, managed := range []string{"/cellar/", "/homebrew/", "/nix/store/", "/usr/local/cellar/"} {
		if strings.Contains(lower, managed) {
			return fmt.Errorf("%s looks managed by a package manager; upgrade it through that instead", exePath)
		}
	}
	// Writability: try opening the directory for write via a temp file.
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".punch-write-test-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (need write access to self-upgrade): %w", dir, err)
	}
	name := tmp.Name()
	tmp.Close()
	os.Remove(name)
	return nil
}

func findAsset(rel *Release, name string) *Asset {
	for i := range rel.Assets {
		if rel.Assets[i].Name == name {
			return &rel.Assets[i]
		}
	}
	return nil
}

// downloadAsset fetches an asset's bytes via the API asset URL (works on
// private GHE repos with the octet-stream Accept header and auth).
func downloadAsset(a *Asset) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, a.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	authHeaders(req)
	resp, err := newClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ChecksumFor parses SHA256SUMS content ("<hex>  <name>" per line) and returns
// the hex checksum for the given file name.
func ChecksumFor(sums, name string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// The name may be prefixed with "*" (binary mode) or "./".
		fn := strings.TrimPrefix(fields[len(fields)-1], "*")
		fn = strings.TrimPrefix(fn, "./")
		if fn == name {
			return fields[0], true
		}
	}
	return "", false
}

// extractBinary reads a .tar.gz and returns the bytes of the entry whose base
// name equals binName.
func extractBinary(targz []byte, binName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(targz))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == binName {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", binName)
}

// replaceExecutable atomically swaps the running binary for newData by writing
// to a temp file in the same directory and renaming over the target.
func replaceExecutable(exePath string, newData []byte) error {
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".punch-upgrade-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if rename succeeded

	if _, err := tmp.Write(newData); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, exePath); err != nil {
		return fmt.Errorf("replace %s: %w", exePath, err)
	}
	return nil
}
