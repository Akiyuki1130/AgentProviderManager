package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// omp 侧认证口径的核心用例：自定义头、authHeader、!command / 环境变量名解析、
// 跨主机重定向剥离，以及 4 个导出入口共用同一份网络实现。

func TestFetchModelsWithOptionsForwardsHeaders(t *testing.T) {
	withPrivateHTTP(t)
	var gotX, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotX = r.Header.Get("X-Foo")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	}))
	t.Cleanup(srv.Close)

	models, err := FetchModelsWithOptions(srv.URL+"/v1", "sk-placeholder", FetchOptions{
		Headers: map[string]string{"X-Foo": "bar"},
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(models) != 1 || models[0].ID != "m1" {
		t.Fatalf("models = %+v", models)
	}
	if gotX != "bar" {
		t.Errorf("provider header not forwarded: %q", gotX)
	}
	if gotAuth != "Bearer sk-placeholder" {
		t.Errorf("native bearer missing: %q", gotAuth)
	}
}

func TestFetchModelsWithOptionsCustomAuthHeaderWins(t *testing.T) {
	withPrivateHTTP(t)
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	}))
	t.Cleanup(srv.Close)

	_, err := FetchModelsWithOptions(srv.URL+"/v1", "sk-placeholder", FetchOptions{
		Headers: map[string]string{"authorization": "Token custom-value"},
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotAuth != "Token custom-value" {
		t.Errorf("custom Authorization must win over native bearer: %q", gotAuth)
	}
}

func TestFetchModelsWithOptionsAuthHeaderFlag(t *testing.T) {
	withPrivateHTTP(t)
	type seen struct{ apiKey, auth string }
	capture := func() (*seen, string) {
		got := &seen{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got.apiKey = r.Header.Get("X-Api-Key")
			got.auth = r.Header.Get("Authorization")
			io.WriteString(w, `{"data":[{"id":"m1"}]}`)
		}))
		t.Cleanup(srv.Close)
		return got, srv.URL + "/v1"
	}

	// anthropic：原生用 x-api-key；authHeader=true 时额外补 Authorization: Bearer。
	got, base := capture()
	yes := true
	if _, err := FetchModelsWithOptions(base, "sk-placeholder", FetchOptions{Kind: "anthropic", AuthHeader: &yes}, 10*time.Second); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.apiKey != "sk-placeholder" || got.auth != "Bearer sk-placeholder" {
		t.Errorf("anthropic authHeader=true: x-api-key=%q authorization=%q", got.apiKey, got.auth)
	}

	// authHeader 未设置：不补 Authorization（保持 omp 语义，false 也不关闭原生认证）。
	got2, base2 := capture()
	if _, err := FetchModelsWithOptions(base2, "sk-placeholder", FetchOptions{Kind: "anthropic"}, 10*time.Second); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got2.apiKey != "sk-placeholder" || got2.auth != "" {
		t.Errorf("anthropic default: x-api-key=%q authorization=%q", got2.apiKey, got2.auth)
	}
}

func TestFetchModelsWithOptionsRejectsCommandKey(t *testing.T) {
	withPrivateHTTP(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	}))
	t.Cleanup(srv.Close)

	_, err := FetchModelsWithOptions(srv.URL+"/v1", "!op read op://vault/key", FetchOptions{}, 10*time.Second)
	mfe, ok := err.(*ModelFetchError)
	if !ok {
		t.Fatalf("err = %v, want *ModelFetchError", err)
	}
	if mfe.ErrorCode != "KEY_COMMAND" {
		t.Errorf("error code = %q, want KEY_COMMAND", mfe.ErrorCode)
	}
	if hits != 0 {
		t.Errorf("command key must reject before any request, hits=%d", hits)
	}
	if strings.Contains(mfe.Msg, "op read") {
		t.Errorf("error message must not echo the command: %q", mfe.Msg)
	}
}

func TestFetchModelsWithOptionsHeaderValueResolution(t *testing.T) {
	withPrivateHTTP(t)
	t.Setenv("APM_OMP_TEST_HEADER", "env-value")
	var gotSecret, gotEnv string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-Secret")
		gotEnv = r.Header.Get("X-Env")
		io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	}))
	t.Cleanup(srv.Close)

	_, err := FetchModelsWithOptions(srv.URL+"/v1", "", FetchOptions{
		Headers: map[string]string{
			"X-Secret": "!echo leaked",
			"X-Env":    "APM_OMP_TEST_HEADER",
		},
	}, 10*time.Second)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotSecret != "" {
		t.Errorf("!command header must not be sent: %q", gotSecret)
	}
	if gotEnv != "env-value" {
		t.Errorf("env header resolution = %q, want env-value", gotEnv)
	}
}

func TestFetchModelsWithOptionsCrossHostStripsCustomAuthHeaders(t *testing.T) {
	withPrivateHTTP(t)
	var gotAuth, gotAPIKey, gotTrace string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-Api-Key")
		gotTrace = r.Header.Get("X-Trace")
		io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	}))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/models", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	const key = "sk-placeholder-xyz"
	if _, err := FetchModelsWithOptions(redirector.URL+"/v1", key, FetchOptions{
		Headers: map[string]string{"X-Api-Key": key, "X-Trace": "plain"},
	}, 10*time.Second); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotAuth != "" || gotAPIKey != "" {
		t.Errorf("cross-host redirect leaked credentials: authorization=%q x-api-key=%q", gotAuth, gotAPIKey)
	}
	if gotTrace != "plain" {
		t.Errorf("non-sensitive header should survive redirect: %q", gotTrace)
	}
}

func TestProbeProviderRequestWithHeaders(t *testing.T) {
	withPrivateHTTP(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Foo") != "bar" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Foo") != "bar" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `{"ok":true}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	report := ProbeProviderRequest(srv.URL+"/v1", "sk-placeholder", ProbeOptions{
		Headers: map[string]string{"X-Foo": "bar"},
		Kind:    "openai-compatible",
	}, 10*time.Second)
	if !report.Models.OK || report.Models.Count != 1 {
		t.Fatalf("models step = %+v", report.Models)
	}
	if report.ChatSkipped || !report.Chat.OK {
		t.Fatalf("chat step = %+v skipped=%v", report.Chat, report.ChatSkipped)
	}
}

func TestProbeProviderRequestCommandKey(t *testing.T) {
	withPrivateHTTP(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("command key must reject before any request")
		io.WriteString(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)

	report := ProbeProviderRequest(srv.URL+"/v1", "!op read op://vault/key", ProbeOptions{Kind: "openai-compatible"}, 10*time.Second)
	if report.Models.OK || report.Models.Code != "KEY_COMMAND" {
		t.Fatalf("models step = %+v", report.Models)
	}
	if !report.ChatSkipped {
		t.Errorf("chat must be skipped after models failure")
	}
}

func TestProviderEditOMPExtraFieldsJSON(t *testing.T) {
	raw, err := json.Marshal(ProviderEdit{
		ID:             "p1",
		ReadOnly:       true,
		ReadOnlyReason: "协议不可建模",
		APIKeyKind:     "env",
		RawAPI:         "google-generative-ai",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, key := range []string{`"read_only":true`, `"read_only_reason":"协议不可建模"`, `"api_key_kind":"env"`, `"raw_api":"google-generative-ai"`} {
		if !strings.Contains(text, key) {
			t.Errorf("payload missing %s: %s", key, text)
		}
	}
	// 其它 harness 不填这四个字段时不得出现在 JSON 里（omitempty）。
	plain, err := json.Marshal(ProviderEdit{ID: "p2"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"read_only", "read_only_reason", "api_key_kind", "raw_api"} {
		if strings.Contains(string(plain), `"`+key+`"`) {
			t.Errorf("empty %s leaked into JSON: %s", key, plain)
		}
	}

	// 摘要与位置字段不带 omitempty：既有前端契约要求键始终存在。
	sum, _ := json.Marshal(ProviderSummary{ReadOnly: true})
	if !strings.Contains(string(sum), `"read_only":true`) {
		t.Errorf("summary read_only missing: %s", sum)
	}
	loc, _ := json.Marshal(ConfigLocation{NeedsMigration: true})
	if !strings.Contains(string(loc), `"needs_migration":true`) {
		t.Errorf("location needs_migration missing: %s", loc)
	}
}
