package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rewriteTransport struct{ serverURL string }

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	server, _ := http.NewRequest(http.MethodGet, t.serverURL, nil)
	clone.URL.Host = server.URL.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func testClient(serverURL string) *http.Client {
	return &http.Client{Transport: rewriteTransport{serverURL: serverURL}}
}

func releaseJSON(tag string, digest string, assetURL string, checksumURL string, size int64) []byte {
	assets := []Asset{{Name: "AgentProviderManager-windows-amd64.exe", BrowserDownloadURL: assetURL, Size: size, Digest: digest}}
	if digest == "" {
		assets = append(assets, Asset{Name: "SHA256SUMS.txt", BrowserDownloadURL: checksumURL})
	}
	data, _ := json.Marshal(Release{TagName: tag, Assets: assets})
	return data
}

func TestCheckLatestAndDownloadWithDigest(t *testing.T) {
	payload := []byte("test executable")
	hash := sha256.Sum256(payload)
	digest := hex.EncodeToString(hash[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Akiyuki1130/AgentProviderManager/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(releaseJSON("v1.2.0", digest, "https://github.com/Akiyuki1130/AgentProviderManager/releases/download/v1.2.0/AgentProviderManager-windows-amd64.exe", "", int64(len(payload))))
		case "/download.exe", "/Akiyuki1130/AgentProviderManager/releases/download/v1.2.0/AgentProviderManager-windows-amd64.exe":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	info, err := CheckLatest(context.Background(), testClient(server.URL), APIBaseURL, "1.1.9")
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsUpdate || info.Version != "1.2.0" || info.ExpectedSHA256 != digest {
		t.Fatalf("unexpected info: %+v", info)
	}

	// Official GitHub URLs are rewritten only by this test transport, so the
	// test performs no real GitHub request.
	dir := t.TempDir()
	updater := &Updater{Client: testClient(server.URL), BaseURL: APIBaseURL, StagingDir: dir}
	staged, err := updater.DownloadLatest(context.Background(), "1.1.9", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(staged.StagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("staged payload %q", got)
	}
	if filepath.Dir(staged.StagedPath) != dir {
		t.Fatalf("staged outside directory: %s", staged.StagedPath)
	}
}

func TestChecksumFallbackAndSecurity(t *testing.T) {
	payload := []byte("checksum executable")
	hash := sha256.Sum256(payload)
	digest := hex.EncodeToString(hash[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/Akiyuki1130/AgentProviderManager/releases/latest":
			_, _ = w.Write(releaseJSON("2.0.0", "", "https://github.com/download.exe", "https://objects.githubusercontent.com/checksums", int64(len(payload))))
		case "/checksums":
			_, _ = w.Write([]byte(digest + " *AgentProviderManager-windows-amd64.exe\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	updater := &Updater{Client: testClient(server.URL), BaseURL: APIBaseURL}
	info, err := updater.CheckLatest(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if info.ExpectedSHA256 != digest {
		t.Fatalf("checksum fallback: %s", info.ExpectedSHA256)
	}

	for _, raw := range []string{"http://api.github.com/x", "https://user:pass@api.github.com/x", "https://localhost/x", "https://192.168.1.1/x", "https://evil.example/x"} {
		if _, err := getResponse(context.Background(), testClient(server.URL), raw, false); err == nil {
			t.Errorf("accepted unsafe URL %q", raw)
		}
	}
}

func TestStableVersionsRejectDraftPrereleaseAndDowngrade(t *testing.T) {
	cases := []struct {
		name    string
		release Release
		current string
		want    string
	}{
		{"draft", Release{TagName: "v2.0.0", Draft: true}, "1.0.0", "draft"},
		{"prerelease", Release{TagName: "v2.0.0-rc.1", Prerelease: true}, "1.0.0", "prerelease"},
		{"downgrade", Release{TagName: "v1.0.0"}, "2.0.0", "older"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := json.Marshal(tc.release)
				_, _ = w.Write(data)
			}))
			defer server.Close()
			_, err := CheckLatest(context.Background(), testClient(server.URL), APIBaseURL, tc.current)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error %v", err)
			}
		})
	}
}
