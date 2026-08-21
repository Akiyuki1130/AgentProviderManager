package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBaseURL(t *testing.T) {
	SetHTTPAllowed(false)
	t.Cleanup(func() { SetHTTPAllowed(false) })
	cases := []struct {
		url     string
		wantOK  bool
		contain string
	}{
		{"", false, "请填写"},
		{"ftp://example.com/v1", false, "仅支持 http/https"},
		{"http://example.com/v1", false, "https"},
		{"https://example.com/v1", true, ""},
		{"http://localhost:8000/v1", false, "https"},
		{"http://127.0.0.1/v1", false, "https"},
		{"https://user:pass@example.com/v1", false, "用户名"},
		{"https://example.com/v1?q=1", false, "查询参数"},
		{"https://", false, ""},
	}
	for _, c := range cases {
		ok, msg := ValidateBaseURL(c.url)
		if ok != c.wantOK {
			t.Errorf("ValidateBaseURL(%q) ok=%v want %v msg=%q", c.url, ok, c.wantOK, msg)
		}
		if !c.wantOK && c.contain != "" && !strings.Contains(msg, c.contain) {
			t.Errorf("ValidateBaseURL(%q) msg=%q want contain %q", c.url, msg, c.contain)
		}
	}
}

func TestValidateBaseURL_HTTPAllowed(t *testing.T) {
	SetHTTPAllowed(true)
	t.Cleanup(func() { SetHTTPAllowed(false) })
	if ok, msg := ValidateBaseURL("http://example.com/v1"); !ok {
		t.Errorf("http should pass when allowed: %q", msg)
	}
	if ok, _ := ValidateBaseURL("https://example.com/v1"); !ok {
		t.Errorf("https should always pass")
	}
	// SSRF checks are unaffected: fetch-time private hosts stay blocked.
	if err := ValidateFetchURL("http://127.0.0.1/v1/models"); err == nil {
		t.Errorf("private host must stay blocked even with http allowed")
	}
	if err := ValidateFetchURL("http://api.example.com/v1/models"); err != nil {
		t.Errorf("public http host should pass when allowed: %v", err)
	}
}

func TestValidateFetchURL_BlocksPrivate(t *testing.T) {
	SetHTTPAllowed(false)
	t.Cleanup(func() { SetHTTPAllowed(false) })
	block := []string{
		"https://localhost/v1/models",
		"https://127.0.0.1/v1/models",
		"https://10.0.0.1/v1/models",
		"https://192.168.1.1/v1/models",
		"http://169.254.1.1/v1/models",
		"http://api.example.com/v1/models",
	}
	for _, u := range block {
		if err := ValidateFetchURL(u); err == nil {
			t.Errorf("ValidateFetchURL(%q) should be blocked", u)
		}
	}
	allow := []string{
		"https://api.openai.com/v1/models",
		"https://example.com/v1/models",
	}
	for _, u := range allow {
		if err := ValidateFetchURL(u); err != nil {
			t.Errorf("ValidateFetchURL(%q) should pass, got %v", u, err)
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	if got := NormalizeBaseURL("https://example.com/v1/models"); got != "https://example.com/v1" {
		t.Errorf("NormalizeBaseURL models suffix: got %q", got)
	}
	if got := NormalizeBaseURL("https://example.com/v1"); got != "https://example.com/v1" {
		t.Errorf("NormalizeBaseURL v1: got %q", got)
	}
}

func TestModelsURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com":           "https://example.com/v1/models",
		"https://example.com/v1":        "https://example.com/v1/models",
		"https://example.com/v2":        "https://example.com/v2/models",
		"https://example.com/v1/models": "https://example.com/v1/models",
	}
	for in, want := range cases {
		if got := ModelsURL(in); got != want {
			t.Errorf("ModelsURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestParseTokens(t *testing.T) {
	cases := []struct {
		input interface{}
		want  *int
	}{
		{"128k", intPtr(128000)},
		{"1m", intPtr(1000000)},
		{4096, intPtr(4096)},
		{"abc", nil},
		{"0", nil},
		{"", nil},
		{nil, nil},
		{true, nil},
	}
	for _, c := range cases {
		got := ParseTokens(c.input)
		if (got == nil) != (c.want == nil) {
			t.Errorf("ParseTokens(%v)=%v want %v", c.input, got, c.want)
		} else if got != nil && *got != *c.want {
			t.Errorf("ParseTokens(%v)=%d want %d", c.input, *got, *c.want)
		}
	}
}

func intPtr(v int) *int { return &v }

func TestGuessReasoning(t *testing.T) {
	if !GuessReasoning("deepseek-r1") {
		t.Error("deepseek-r1 should be reasoning")
	}
	if GuessReasoning("gpt-4o") {
		t.Error("gpt-4o should not be reasoning")
	}
}

func TestAutoProviderID(t *testing.T) {
	if got := AutoProviderID("https://api.openai.com/v1"); got != "openai" {
		t.Errorf("AutoProviderID got %q want openai", got)
	}
	if got := AutoProviderID(""); got != "" {
		t.Errorf("AutoProviderID empty got %q", got)
	}
}

func TestInferKind(t *testing.T) {
	if got := InferKind("https://api.example.com/anthropic/v1"); got != "anthropic" {
		t.Errorf("InferKind anthropic got %q", got)
	}
	if got := InferKind("https://api.openai.com/v1"); got != "openai-compatible" {
		t.Errorf("InferKind openai got %q", got)
	}
}

func TestBuildProviderCfg(t *testing.T) {
	cards := []ModelCard{{ModelID: "gpt-4o", Name: "gpt-4o", Context: "128k", Output: "8k", Select: true}}
	pid, cfg, _, err := BuildProviderCfg("test-provider", "Test", "https://api.example.com/v1", "sk-123", cards, "openai-compatible")
	if err != nil {
		t.Fatalf("BuildProviderCfg: %v", err)
	}
	if pid != "test-provider" {
		t.Errorf("pid=%q", pid)
	}
	if cfg["kind"] != "openai-compatible" {
		t.Errorf("kind=%v", cfg["kind"])
	}
	// responses should write as openai
	_, cfg2, _, err := BuildProviderCfg("p2", "P2", "https://x.example.com/v1", "k", cards, "responses")
	if err != nil {
		t.Fatalf("BuildProviderCfg responses: %v", err)
	}
	if cfg2["kind"] != "openai" {
		t.Errorf("responses kind should be openai, got %v", cfg2["kind"])
	}
}

func TestProviderSummaryAndEdit(t *testing.T) {
	cfg := map[string]interface{}{
		"provider": map[string]interface{}{
			"my-provider": map[string]interface{}{
				"name":    "My Provider",
				"kind":    "openai-compatible",
				"options": map[string]interface{}{"baseURL": "https://api.example.com/v1", "apiKey": "sk-123"},
				"models":  map[string]interface{}{"gpt-4o": map[string]interface{}{"name": "gpt-4o"}},
			},
		},
	}
	summaries := BuildProviderSummary(cfg)
	if len(summaries) != 1 || summaries[0].ID != "my-provider" {
		t.Fatalf("BuildProviderSummary: %+v", summaries)
	}
	edit, err := ProviderToEdit(cfg, "my-provider")
	if err != nil {
		t.Fatalf("ProviderToEdit: %v", err)
	}
	if edit.ID != "my-provider" || len(edit.Cards) != 1 {
		t.Errorf("ProviderToEdit: %+v", edit)
	}
}

func TestNormalizeConfigKinds(t *testing.T) {
	cfg := map[string]interface{}{
		"provider": map[string]interface{}{
			"r":  map[string]interface{}{"kind": "responses"},
			"ok": map[string]interface{}{"kind": "anthropic"},
		},
	}
	changed := NormalizeConfigKinds(cfg)
	if !changed {
		t.Error("should be changed")
	}
	providers := cfg["provider"].(map[string]interface{})
	if providers["r"].(map[string]interface{})["kind"] != "openai" {
		t.Errorf("responses should become openai, got %v", providers["r"])
	}
}

func TestLoadAndWriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Missing file returns empty
	m, err := LoadConfig(path)
	if err != nil || len(m) != 0 {
		t.Fatalf("LoadConfig missing: %v %+v", err, m)
	}
	// Write and read back
	orig := map[string]interface{}{"provider": map[string]interface{}{"p": map[string]interface{}{"name": "P"}}}
	if err := WriteConfig(path, orig, ""); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := loaded["provider"]; !ok {
		t.Error("provider missing after write")
	}
	// JSONC with comments
	jsoncPath := filepath.Join(dir, "config2.jsonc")
	_ = os.WriteFile(jsoncPath, []byte(`{
		// comment
		"provider": {
			"p": {
				"options": {"baseURL": "https://api.example.com/v1",}, // trailing comma
			},
		},
	}`), 0644)
	m2, err := LoadConfig(jsoncPath)
	if err != nil {
		t.Fatalf("LoadConfig JSONC: %v", err)
	}
	if _, ok := m2["provider"]; !ok {
		t.Error("provider missing from JSONC")
	}
	// Fingerprint check
	_, fp, err := LoadConfigWithFingerprint(path)
	if err != nil {
		t.Fatalf("LoadConfigWithFingerprint: %v", err)
	}
	_ = os.WriteFile(path, []byte(`{"provider":{}}`), 0644)
	if err := WriteConfig(path, orig, fp); err == nil {
		t.Error("WriteConfig should fail on stale fingerprint")
	}
}

func TestBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = os.WriteFile(path, []byte(`{"provider":{"a":{"name":"A"}}}`), 0644)
	bak, err := BackupConfig(path)
	if err != nil || bak == "" {
		t.Fatalf("BackupConfig: %v %q", err, bak)
	}
	// Modify and restore
	_ = os.WriteFile(path, []byte(`{"provider":{"b":{"name":"B"}}}`), 0644)
	snap, err := RestoreBackup(path, bak)
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	_ = snap
	data, _ := os.ReadFile(path)
	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)
	providers := m["provider"].(map[string]interface{})
	if _, ok := providers["a"]; !ok {
		t.Error("restore should bring back provider a")
	}
}

func TestKeychainValidate(t *testing.T) {
	ok, msg := ValidateKeychainEntry(KeychainEntry{Name: "", APIKey: ""})
	if ok {
		t.Error("empty should fail")
	}
	_ = msg
	ok, _ = ValidateKeychainEntry(KeychainEntry{Name: "test", APIKey: "sk-123"})
	if !ok {
		t.Error("valid should pass")
	}
}

func TestDetectConfigLocations(t *testing.T) {
	locs := DetectConfigLocations()
	if len(locs) == 0 {
		t.Error("should have at least one location")
	}
	for _, loc := range locs {
		if loc.Path == "" {
			t.Error("location path empty")
		}
	}
}

func TestRedactSensitive(t *testing.T) {
	s := RedactSensitive("Bearer sk-12345 and api_key: secret123", []string{"secret123"})
	if strings.Contains(s, "secret123") {
		t.Error("should redact secret")
	}
	if strings.Contains(s, "sk-12345") {
		t.Error("should redact bearer")
	}
}

func TestOpencodePreviewAndMerge(t *testing.T) {
	dir := t.TempDir()
	ocPath := filepath.Join(dir, "opencode.json")
	_ = os.WriteFile(ocPath, []byte(`{"provider":{"my-provider":{"name":"My","options":{"baseURL":"https://api.example.com/v1","apiKey":"sk-123"},"models":{"gpt-4o":{"name":"gpt-4o"}}}}}`), 0644)
	preview := OpencodeImportPreview(ocPath)
	if len(preview.Providers) != 1 {
		t.Fatalf("OpencodeImportPreview: %+v", preview)
	}
	// Import into empty zcode config
	zcodeCfg := map[string]interface{}{"provider": map[string]interface{}{}}
	ocCfg, _ := LoadConfig(ocPath)
	result, imported, _, err := ImportOpencodeProviders(zcodeCfg, ocCfg, []string{"my-provider"}, true)
	if err != nil {
		t.Fatalf("ImportOpencodeProviders: %v", err)
	}
	if len(imported) != 1 {
		t.Errorf("imported=%v", imported)
	}
	if _, ok := result["provider"].(map[string]interface{})["my-provider"]; !ok {
		t.Error("provider not imported")
	}
}

func TestNativeProviderFormats(t *testing.T) {
	ocRaw := map[string]interface{}{"name": "Demo", "npm": "@ai-sdk/openai-compatible", "options": map[string]interface{}{"baseURL": "https://api.example.com/v1"}, "models": map[string]interface{}{"r1": map[string]interface{}{"id": "r1", "reasoning": true, "temperature": true, "tool_call": true, "attachment": true, "headers": map[string]interface{}{"x-test": "1"}, "options": map[string]interface{}{"reasoningEffort": "high"}, "limit": map[string]interface{}{"context": 128000, "output": 4096}, "modalities": map[string]interface{}{"input": []interface{}{"text"}, "output": []interface{}{"text"}}, "variants": map[string]interface{}{"high": map[string]interface{}{}}}}}
	pid, internal, err := ConvertOpencodeProvider("demo", ocRaw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pid != "demo" {
		t.Fatal(pid)
	}
	native := OpenCodeProviderFromCfg(internal)
	model := native["models"].(map[string]interface{})["r1"].(map[string]interface{})
	if _, ok := model["reasoning"].(bool); !ok {
		t.Fatalf("reasoning is not bool: %#v", model["reasoning"])
	}
	if _, ok := model["variants"].(map[string]interface{}); !ok {
		t.Fatal("variants missing")
	}
	if model["temperature"] != true || model["tool_call"] != true || model["attachment"] != true {
		t.Fatalf("capabilities lost: %#v", model)
	}
	if model["headers"].(map[string]interface{})["x-test"] != "1" {
		t.Fatal("headers lost")
	}
	if model["options"].(map[string]interface{})["reasoningEffort"] != "high" {
		t.Fatal("options lost")
	}
	if _, ok := native["kind"]; ok {
		t.Fatal("private kind leaked")
	}

	dsh := map[string]interface{}{"id": "r1", "name": "R1", "contextWindow": 128000, "maxTokens": 4096, "reasoningEfforts": map[string]interface{}{"off": nil, "high": "high"}}
	card := CfgToCard("r1", ConvertDeepSeekModel("r1", dsh, "openai-compatible"))
	out := dshModelToRaw(card, "openai-compatible")
	if out["contextWindow"] != 128000 || out["maxTokens"] != 4096 {
		t.Fatalf("limits lost: %#v", out)
	}
	if _, ok := out["reasoningEfforts"].(map[string]interface{}); !ok {
		t.Fatal("reasoningEfforts missing")
	}
}

func TestFetchModelsRealServer(t *testing.T) {
	// Local test server to avoid network dependency
	// Use non-private host via hosts trick: 127.0.0.1 is blocked, so we test http failure path
	_, err := FetchModelsRaw("https://127.0.0.1/v1", "sk-test", 2000000000)
	if err == nil {
		t.Error("127.0.0.1 should be blocked")
	}
}
