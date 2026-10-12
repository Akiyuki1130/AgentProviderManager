package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestApplyRedirectAuth(t *testing.T) {
	cases := []struct {
		name       string
		nextHost   string
		apiKey     string
		start      map[string]string
		wantHeader string // "" 表示不应存在 Authorization 头
	}{
		{"同主机且 Key 非空：保留", "api.example.com", "sk-placeholder", map[string]string{"Authorization": "Bearer sk-placeholder"}, "Bearer sk-placeholder"},
		{"同主机但 Key 为空：不写空 Bearer", "api.example.com", "", map[string]string{}, ""},
		{"同主机且 Key 为空：清掉旧头", "api.example.com", "", map[string]string{"Authorization": "Bearer sk-placeholder"}, ""},
		{"跨主机：删除凭据", "evil.example.com", "sk-placeholder", map[string]string{"Authorization": "Bearer sk-placeholder"}, ""},
		{"跨主机且 Key 为空：保持为空", "evil.example.com", "", map[string]string{}, ""},
	}
	for _, tc := range cases {
		headers := tc.start
		applyRedirectAuth(headers, tc.nextHost, "api.example.com", tc.apiKey)
		got, ok := headers["Authorization"]
		if tc.wantHeader == "" {
			if ok {
				t.Errorf("%s：不应保留 Authorization，实际 %q", tc.name, got)
			}
			continue
		}
		if !ok || got != tc.wantHeader {
			t.Errorf("%s：Authorization = %q (存在=%v)，want %q", tc.name, got, ok, tc.wantHeader)
		}
	}
}

// withPrivateHTTP 打开测试所需的 http/内网地址开关，并在结束时恢复。
func withPrivateHTTP(t *testing.T) {
	t.Helper()
	prevHTTP, prevPrivate := HTTPAllowed(), PrivateHostsAllowed()
	SetHTTPAllowed(true)
	SetPrivateHostsAllowed(true)
	t.Cleanup(func() {
		SetHTTPAllowed(prevHTTP)
		SetPrivateHostsAllowed(prevPrivate)
	})
}

func TestFetchModelsRawRedirectToOtherHostDropsAuth(t *testing.T) {
	withPrivateHTTP(t)

	var mu sync.Mutex
	var targetAuth string
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		targetAuth = r.Header.Get("Authorization")
		targetCalls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/models", http.StatusFound)
	}))
	defer origin.Close()

	models, err := FetchModelsRaw(origin.URL+"/v1", "sk-placeholder", 10*time.Second)
	if err != nil {
		t.Fatalf("FetchModelsRaw: %v", err)
	}
	if len(models) != 1 || models[0].ID != "m1" {
		t.Fatalf("models = %#v", models)
	}
	mu.Lock()
	defer mu.Unlock()
	if targetCalls != 1 {
		t.Fatalf("目标主机被请求 %d 次，want 1", targetCalls)
	}
	if targetAuth != "" {
		t.Fatalf("跨主机重定向不应转发 Authorization，实际 %q", targetAuth)
	}
}

func TestFetchModelsRawSameHostRedirectDoesNotSendEmptyBearer(t *testing.T) {
	withPrivateHTTP(t)

	var mu sync.Mutex
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			http.Redirect(w, r, "/v1/models/", http.StatusFound)
			return
		}
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":["m1","m2"]}`)
	}))
	defer srv.Close()

	models, err := FetchModelsRaw(srv.URL+"/v1", "", 10*time.Second)
	if err != nil {
		t.Fatalf("FetchModelsRaw: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v", models)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(auths) != 1 {
		t.Fatalf("重定向后的请求数 = %d，want 1", len(auths))
	}
	if auths[0] != "" {
		t.Fatalf("空 Key 的重定向请求不应带 Authorization，实际 %q", auths[0])
	}
}
