package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newProbeTestServer 提供 /v1/models 与三种 kind 的 chat 端点：
// 凭据正确回 200，否则回 401；并记录 anthropic 请求看到的 x-api-key。
func newProbeTestServer(t *testing.T) (string, *string) {
	t.Helper()
	seenAnthropicKey := new(string)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"id":"m1"},{"id":"m2"}]}`)
	})
	chat := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/messages" {
			*seenAnthropicKey = r.Header.Get("x-api-key")
		}
		ok := r.Header.Get("Authorization") == "Bearer sk-placeholder"
		if r.URL.Path == "/v1/messages" {
			ok = *seenAnthropicKey == "sk-placeholder"
		}
		w.Header().Set("Content-Type", "application/json")
		if ok {
			io.WriteString(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"unauthorized"}}`)
	}
	mux.HandleFunc("/v1/chat/completions", chat)
	mux.HandleFunc("/v1/messages", chat)
	mux.HandleFunc("/v1/responses", chat)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/v1", seenAnthropicKey
}

func unlockLocalHTTP(t *testing.T) {
	t.Helper()
	SetHTTPAllowed(true)
	SetPrivateHostsAllowed(true)
	t.Cleanup(func() { SetHTTPAllowed(false); SetPrivateHostsAllowed(false) })
}

func TestProbeChatKinds(t *testing.T) {
	unlockLocalHTTP(t)
	baseURL, seenAnthropicKey := newProbeTestServer(t)
	for _, kind := range []string{"openai-compatible", "anthropic", "responses"} {
		report := TestConnection(baseURL, "sk-placeholder", kind)
		if !report.Models.OK || report.Models.Count != 2 {
			t.Fatalf("%s: models step = %+v", kind, report.Models)
		}
		if report.ChatSkipped || !report.Chat.OK {
			t.Fatalf("%s: chat step = %+v skipped=%v", kind, report.Chat, report.ChatSkipped)
		}
		if kind == "anthropic" && *seenAnthropicKey != "sk-placeholder" {
			t.Errorf("anthropic: server saw x-api-key %q", *seenAnthropicKey)
		}
	}
	// 错误凭据：401 且报错文本不含 API Key。
	report := TestConnection(baseURL, "sk-placeholder-wrong", "openai-compatible")
	if report.Chat.OK || report.Chat.Code != "HTTP_401" {
		t.Fatalf("wrong key: chat step = %+v", report.Chat)
	}
	if strings.Contains(report.Chat.Error, "sk-placeholder-wrong") {
		t.Errorf("error message leaked key: %q", report.Chat.Error)
	}
}

func TestProbeModelsFailSkipsChat(t *testing.T) {
	unlockLocalHTTP(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	report := TestConnection(srv.URL+"/v1", "sk-placeholder", "openai-compatible")
	if report.Models.OK || report.Models.Code != "HTTP_404" {
		t.Fatalf("models step = %+v", report.Models)
	}
	if !report.ChatSkipped {
		t.Errorf("chat should be skipped after models failure")
	}
}
