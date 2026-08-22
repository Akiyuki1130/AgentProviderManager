// Package update implements the network and installation primitives used by the
// application updater. It deliberately has no dependency on the Wails layer.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	RepositoryOwner = "Akiyuki1130"
	RepositoryName  = "AgentProviderManager"
	APIBaseURL      = "https://api.github.com"

	maxReleaseJSON = 2 << 20
	maxChecksums   = 4 << 20
	maxExecutable  = 500 << 20
	requestTimeout = 90 * time.Second
)

var (
	ErrNoUpdate          = errors.New("no update available")
	ErrDowngrade         = errors.New("latest release is older than current version")
	ErrUnsupportedHelper = errors.New("update helper is only supported on Windows")
)

// Asset is the subset of a GitHub release asset needed by the updater.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

// Release describes the GitHub latest-release response.
type Release struct {
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	HTMLURL    string  `json:"html_url"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// UpdateInfo is the result of checking or staging an update.
type UpdateInfo struct {
	Release        Release
	Asset          Asset
	Version        string
	ExpectedSHA256 string
	IsUpdate       bool
	StagedPath     string
	ExecutablePath string
	Bytes          int64
}

// ProgressFunc receives downloaded bytes and the total, if known.
type ProgressFunc func(downloaded, total int64)

// Updater makes the HTTP client and API endpoint injectable without making
// production callers able to select a repository or an arbitrary download host.
type Updater struct {
	Client     *http.Client
	BaseURL    string
	StagingDir string
}

// New returns an updater using the fixed GitHub API endpoint.
func New(client *http.Client) *Updater { return &Updater{Client: client, BaseURL: APIBaseURL} }

func (u *Updater) client() *http.Client {
	if u != nil && u.Client != nil {
		c := *u.Client
		return &c
	}
	return &http.Client{}
}

func (u *Updater) baseURL() string {
	if u == nil || strings.TrimSpace(u.BaseURL) == "" {
		return APIBaseURL
	}
	return u.BaseURL
}

// CheckLatest checks the fixed repository's formal latest release. A custom
// base URL is useful in tests when the injected client's transport maps the
// fixed api.github.com URL to an httptest server; it is still strictly
// validated as the GitHub API host.
func (u *Updater) CheckLatest(ctx context.Context, currentVersion string) (UpdateInfo, error) {
	base, err := validateAPIBaseURL(u.baseURL())
	if err != nil {
		return UpdateInfo{}, err
	}
	current, err := parseVersion(currentVersion)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("current version: %w", err)
	}
	client := safeClient(u.client(), false)
	releaseURL := strings.TrimRight(base.String(), "/") + "/repos/" + RepositoryOwner + "/" + RepositoryName + "/releases/latest"
	var release Release
	if err := getJSON(ctx, client, releaseURL, &release); err != nil {
		return UpdateInfo{}, err
	}
	if release.Draft || release.Prerelease {
		return UpdateInfo{}, errors.New("GitHub latest release is draft or prerelease")
	}
	latest, err := parseVersion(release.TagName)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("release tag: %w", err)
	}
	if latest.pre != "" {
		return UpdateInfo{}, errors.New("stable release has a prerelease tag")
	}
	cmp := compareVersion(latest, current)
	if cmp < 0 {
		return UpdateInfo{}, ErrDowngrade
	}
	asset, ok := chooseWindowsAMD64Asset(release.Assets)
	if !ok {
		return UpdateInfo{}, errors.New("latest release has no Windows amd64 .exe asset")
	}
	if asset.Size < 0 || asset.Size > maxExecutable {
		return UpdateInfo{}, errors.New("Windows executable exceeds the configured size limit")
	}
	var digest string
	if asset.Digest != "" {
		digest, err = normalizeDigest(asset.Digest)
		if err != nil {
			return UpdateInfo{}, fmt.Errorf("asset digest: %w", err)
		}
	} else {
		digest, err = checksumForAsset(ctx, client, release.Assets, asset.Name)
		if err != nil {
			return UpdateInfo{}, err
		}
	}
	return UpdateInfo{Release: release, Asset: asset, Version: latest.String(), ExpectedSHA256: digest, IsUpdate: cmp > 0}, nil
}

// CheckLatest is the package-level convenience form. The endpoint and client
// remain explicit so tests can inject an HTTP transport without real network I/O.
func CheckLatest(ctx context.Context, client *http.Client, baseURL, currentVersion string) (UpdateInfo, error) {
	return (&Updater{Client: client, BaseURL: baseURL}).CheckLatest(ctx, currentVersion)
}

// DownloadLatest rechecks the release, verifies it, and writes a uniquely
// named staged file. It never replaces executablePath.
func (u *Updater) DownloadLatest(ctx context.Context, currentVersion string, progress ProgressFunc) (info UpdateInfo, err error) {
	info, err = u.CheckLatest(ctx, currentVersion)
	if err != nil {
		return UpdateInfo{}, err
	}
	if !info.IsUpdate {
		return info, ErrNoUpdate
	}
	stagingDir := u.StagingDir
	if stagingDir == "" {
		stagingDir, err = executableDirectory()
		if err != nil {
			return UpdateInfo{}, err
		}
	}
	stagingDir, err = filepath.Abs(stagingDir)
	if err != nil {
		return UpdateInfo{}, err
	}
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return UpdateInfo{}, fmt.Errorf("create staging directory: %w", err)
	}
	f, err := os.CreateTemp(stagingDir, ".apm-update-*.exe")
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("create staged file: %w", err)
	}
	staged := f.Name()
	keepStaged := false
	defer func() {
		if !keepStaged {
			_ = os.Remove(staged)
		}
	}()
	client := safeClient(u.client(), true)
	resp, err := getResponse(ctx, client, info.Asset.BrowserDownloadURL, true)
	if err != nil {
		_ = f.Close()
		return UpdateInfo{}, err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxExecutable+1)
	hash := sha256.New()
	reader := io.TeeReader(limited, hash)
	var downloaded int64
	buf := make([]byte, 128*1024)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				_ = f.Close()
				return UpdateInfo{}, fmt.Errorf("write staged file: %w", err)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, info.Asset.Size)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = f.Close()
			return UpdateInfo{}, fmt.Errorf("download executable: %w", readErr)
		}
	}
	if downloaded > maxExecutable || (info.Asset.Size > 0 && downloaded != info.Asset.Size) {
		_ = f.Close()
		return UpdateInfo{}, errors.New("downloaded executable has an invalid size")
	}
	if err := f.Close(); err != nil {
		return UpdateInfo{}, fmt.Errorf("close staged file: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, info.ExpectedSHA256) {
		return UpdateInfo{}, fmt.Errorf("SHA-256 mismatch: got %s", actual)
	}
	info.StagedPath, err = filepath.Abs(staged)
	if err != nil {
		return UpdateInfo{}, err
	}
	info.Bytes = downloaded
	keepStaged = true
	return info, nil
}

// DownloadLatest is the package-level convenience form.
func DownloadLatest(ctx context.Context, client *http.Client, baseURL, currentVersion, stagingDir string, progress ProgressFunc) (UpdateInfo, error) {
	return (&Updater{Client: client, BaseURL: baseURL, StagingDir: stagingDir}).DownloadLatest(ctx, currentVersion, progress)
}

func executableDirectory() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	return filepath.Dir(path), nil
}

func getJSON(ctx context.Context, client *http.Client, rawURL string, dst any) error {
	resp, err := getResponse(ctx, client, rawURL, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseJSON+1))
	if err != nil {
		return fmt.Errorf("read release response: %w", err)
	}
	if len(data) > maxReleaseJSON {
		return errors.New("release response exceeds size limit")
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decode release response: %w", err)
	}
	return nil
}

func checksumForAsset(ctx context.Context, client *http.Client, assets []Asset, name string) (string, error) {
	var sums *Asset
	for i := range assets {
		if strings.EqualFold(filepath.Base(assets[i].Name), "SHA256SUMS.txt") {
			if sums != nil {
				return "", errors.New("release contains multiple SHA256SUMS.txt assets")
			}
			sums = &assets[i]
		}
	}
	if sums == nil {
		return "", errors.New("release asset has no GitHub digest or SHA256SUMS.txt")
	}
	resp, err := getResponse(ctx, client, sums.BrowserDownloadURL, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksums+1))
	if err != nil {
		return "", fmt.Errorf("read checksum file: %w", err)
	}
	if len(data) > maxChecksums {
		return "", errors.New("checksum file exceeds size limit")
	}
	var found string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !isHexDigest(fields[0]) {
			continue
		}
		candidate := strings.TrimPrefix(fields[1], "*")
		if filepath.Base(candidate) == filepath.Base(name) {
			if found != "" && !strings.EqualFold(found, fields[0]) {
				return "", errors.New("conflicting checksums for executable")
			}
			found = strings.ToLower(fields[0])
		}
	}
	if found == "" {
		return "", errors.New("checksum file has no checksum for executable")
	}
	return found, nil
}

func chooseWindowsAMD64Asset(assets []Asset) (Asset, bool) {
	type candidate struct {
		asset Asset
		score int
	}
	var candidates []candidate
	for _, asset := range assets {
		name := strings.ToLower(filepath.Base(asset.Name))
		if filepath.Base(asset.Name) != asset.Name || strings.ContainsAny(asset.Name, `/\\`) || !strings.HasSuffix(name, ".exe") {
			continue
		}
		if strings.Contains(name, "arm") || strings.Contains(name, "386") || strings.Contains(name, "x86-") || strings.Contains(name, "linux") || strings.Contains(name, "darwin") || strings.Contains(name, "macos") {
			continue
		}
		score := 0
		if strings.Contains(name, "windows") {
			score += 4
		} else if strings.Contains(name, "win") {
			score += 2
		} else {
			continue
		}
		if strings.Contains(name, "amd64") {
			score += 4
		} else if strings.Contains(name, "x86_64") || strings.Contains(name, "x64") {
			score += 3
		} else {
			continue
		}
		candidates = append(candidates, candidate{asset, score})
	}
	if len(candidates) == 0 {
		return Asset{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].asset.Name < candidates[j].asset.Name
	})
	return candidates[0].asset, true
}

var digestPattern = regexp.MustCompile(`^(?i:[0-9a-f]{64})$`)

func isHexDigest(s string) bool { return digestPattern.MatchString(s) }

func normalizeDigest(s string) (string, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(s), "sha256:"))
	if !isHexDigest(s) {
		return "", errors.New("digest is not a SHA-256 value")
	}
	return s, nil
}

type guardedTransport struct {
	base     http.RoundTripper
	download bool
}

func (t guardedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// getResponse and CheckRedirect perform the operation-specific check;
	// this second guard covers custom transports and every RoundTrip.
	if err := validateGitHubURL(req.URL); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func safeClient(in *http.Client, download bool) *http.Client {
	c := *in
	if c.Timeout == 0 {
		c.Timeout = requestTimeout
	}
	var base http.RoundTripper = c.Transport
	if transport, ok := base.(*http.Transport); ok {
		clone := transport.Clone()
		clone.DialContext = safeDialContext(clone.DialContext)
		base = clone
	} else if base == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = safeDialContext(transport.DialContext)
		base = transport
	}
	c.Transport = guardedTransport{base: base, download: download}
	oldRedirect := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if err := validateRequestURL(req.URL, download); err != nil {
			return err
		}
		if oldRedirect != nil {
			return oldRedirect(req, via)
		}
		return nil
	}
	return &c
}

func safeDialContext(_ func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: requestTimeout}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid network address: %w", err)
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve GitHub host: %w", err)
		}
		var lastErr error
		for _, ip := range ips {
			if isForbiddenHost(ip.String()) {
				return nil, fmt.Errorf("resolved GitHub host to forbidden address %s", ip)
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr != nil {
			return nil, fmt.Errorf("connect to GitHub host: %w", lastErr)
		}
		return nil, errors.New("GitHub host has no resolved addresses")
	}
}

func getResponse(ctx context.Context, client *http.Client, rawURL string, download bool) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if err := validateRequestURL(u, download); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", u.Redacted(), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: HTTP %s", u.Redacted(), resp.Status)
	}
	return resp, nil
}

func validateAPIBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid API base URL: %w", err)
	}
	if err := validateURLParts(u, map[string]bool{"api.github.com": true}); err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("API base URL must not contain query or fragment")
	}
	return u, nil
}

func validateGitHubURL(u *url.URL) error {
	return validateURLParts(u, map[string]bool{
		"api.github.com":                        true,
		"github.com":                            true,
		"release-assets.githubusercontent.com":  true,
		"objects.githubusercontent.com":         true,
		"objects-origin.githubusercontent.com":  true,
		"github-releases.githubusercontent.com": true,
	})
}

func validateRequestURL(u *url.URL, download bool) error {
	hosts := map[string]bool{"api.github.com": true}
	if download {
		hosts = map[string]bool{
			"github.com":                            true,
			"release-assets.githubusercontent.com":  true,
			"objects.githubusercontent.com":         true,
			"objects-origin.githubusercontent.com":  true,
			"github-releases.githubusercontent.com": true,
		}
	}
	return validateURLParts(u, hosts)
}

func validateURLParts(u *url.URL, allowed map[string]bool) error {
	if u == nil || u.Scheme != "https" {
		return errors.New("only HTTPS URLs are allowed")
	}
	if u.User != nil {
		return errors.New("URL user information is forbidden")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || !allowed[host] {
		return fmt.Errorf("URL host %q is not an allowed GitHub host", u.Hostname())
	}
	if net.ParseIP(host) != nil || isForbiddenHost(host) {
		return fmt.Errorf("URL host %q is not routable", host)
	}
	if u.Port() != "" && u.Port() != "443" {
		return errors.New("HTTPS URL must use port 443")
	}
	return nil
}

func isForbiddenHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()
}

// semVersion is intentionally small and stable: comparison follows SemVer 2.0.
type semVersion struct {
	major, minor, patch int64
	pre                 string
	build               string
}

var (
	versionPattern    = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$`)
	identifierPattern = regexp.MustCompile(`^[0-9A-Za-z-]+$`)
)

func parseVersion(raw string) (semVersion, error) {
	m := versionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return semVersion{}, fmt.Errorf("invalid semantic version %q", raw)
	}
	major, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return semVersion{}, fmt.Errorf("invalid major version in %q", raw)
	}
	minor, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return semVersion{}, fmt.Errorf("invalid minor version in %q", raw)
	}
	patch, err := strconv.ParseInt(m[3], 10, 64)
	if err != nil {
		return semVersion{}, fmt.Errorf("invalid patch version in %q", raw)
	}
	for _, part := range []struct{ value, name string }{{m[4], "prerelease"}, {m[5], "build"}} {
		if part.value == "" {
			continue
		}
		for _, id := range strings.Split(part.value, ".") {
			if id == "" || !identifierPattern.MatchString(id) || (part.name == "prerelease" && len(id) > 1 && id[0] == '0' && allDigits(id)) {
				return semVersion{}, fmt.Errorf("invalid %s identifier in %q", part.name, raw)
			}
		}
	}
	return semVersion{major, minor, patch, m[4], m[5]}, nil
}

func allDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return value != ""
}

func compareVersion(a, b semVersion) int {
	for _, pair := range [][2]int64{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if a.pre == b.pre {
		return 0
	}
	if a.pre == "" {
		return 1
	}
	if b.pre == "" {
		return -1
	}
	ai, bi := strings.Split(a.pre, "."), strings.Split(b.pre, ".")
	for i := 0; i < len(ai) && i < len(bi); i++ {
		aNum, aErr := strconv.ParseUint(ai[i], 10, 64)
		bNum, bErr := strconv.ParseUint(bi[i], 10, 64)
		if aErr == nil && bErr == nil && aNum != bNum {
			if aNum < bNum {
				return -1
			}
			return 1
		}
		if aErr == nil && bErr != nil {
			return -1
		}
		if aErr != nil && bErr == nil {
			return 1
		}
		if ai[i] != bi[i] {
			if ai[i] < bi[i] {
				return -1
			}
			return 1
		}
	}
	if len(ai) < len(bi) {
		return -1
	}
	return 1
}

func (v semVersion) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
	if v.pre != "" {
		s += "-" + v.pre
	}
	if v.build != "" {
		s += "+" + v.build
	}
	return s
}
