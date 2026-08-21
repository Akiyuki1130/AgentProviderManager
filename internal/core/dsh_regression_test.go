package core

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDeepSeekNativeProviderToEdit(t *testing.T) {
	raw := map[string]interface{}{
		"api": "openai-completions", "baseURL": "https://api.example.com/v1", "displayName": "demo",
		"models": []interface{}{map[string]interface{}{"id": "demo-reasoner", "contextWindow": 128000, "maxTokens": 4096, "reasoningEfforts": map[string]interface{}{"off": nil, "high": "high"}}},
	}
	edit, err := DeepSeekProviderToEdit("demo", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(edit.Cards) != 1 {
		t.Fatalf("cards=%d", len(edit.Cards))
	}
	if edit.Cards[0].ModelID != "demo-reasoner" {
		t.Fatal(edit.Cards[0].ModelID)
	}
	if edit.Cards[0].Context != 128000.0 && edit.Cards[0].Context != 128000 {
		t.Fatalf("context=%#v", edit.Cards[0].Context)
	}
}

func TestDeepSeekReferenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	cfg := map[string]interface{}{
		"agent-default-model": map[string]interface{}{"provider": "sensenova", "model": "deepseek-v4-flash"},
		"llm-pi-ai": map[string]interface{}{"providers": map[string]interface{}{
			"sensenova": map[string]interface{}{"apiKeyEnv": "SENSENOVA_API_KEY", "api": "openai-completions", "baseURL": "https://token.sensenova.cn/v1", "models": []interface{}{
				map[string]interface{}{"id": "sensenova-6.7-flash-lite", "name": "lite", "contextWindow": 262144},
				map[string]interface{}{"id": "deepseek-v4-flash", "contextWindow": 1048576, "customMetadata": "keep"},
			}},
		}},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := DeepSeekLoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := DeepSeekProvidersMap(loaded)["sensenova"].(map[string]interface{})
	_, normalized, err := ConvertDeepSeekProvider("sensenova", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := DeepSeekSaveProviderInConfig(path, loaded, "sensenova", normalized, ""); err != nil {
		t.Fatal(err)
	}
	out, err := DeepSeekLoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := DeepSeekProvidersMap(out)["sensenova"].(map[string]interface{})
	if p["apiKeyEnv"] != "SENSENOVA_API_KEY" {
		t.Fatalf("apiKeyEnv changed: %#v", p["apiKeyEnv"])
	}
	models, _ := p["models"].([]interface{})
	if len(models) != 2 {
		t.Fatalf("models=%d", len(models))
	}
	first, _ := models[0].(map[string]interface{})
	if first["id"] != "sensenova-6.7-flash-lite" {
		t.Fatalf("order changed: %#v", first["id"])
	}
	second, _ := models[1].(map[string]interface{})
	if second["customMetadata"] != "keep" {
		t.Fatalf("custom field lost: %#v", second)
	}
}

func TestDeepSeekCredentialsReadFormats(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "legacy.yaml")
	if err := os.WriteFile(legacyPath, []byte("LEGACY_KEY: legacy-value\nversion: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	legacy := loadCredentials(legacyPath)
	if legacy["LEGACY_KEY"] != "legacy-value" {
		t.Fatalf("legacy credential missing: %#v", legacy)
	}
	if _, ok := legacy["version"]; ok {
		t.Fatal("metadata exposed as credential")
	}

	officialPath := filepath.Join(dir, "official.yaml")
	if err := os.WriteFile(officialPath, []byte("version: 1\nrefs:\n  OFFICIAL_KEY: official-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	official := loadCredentials(officialPath)
	if official["OFFICIAL_KEY"] != "official-value" {
		t.Fatalf("official credential missing: %#v", official)
	}
	if _, ok := official["refs"]; ok {
		t.Fatal("refs metadata exposed as credential")
	}
}

func TestDeepSeekCredentialsWritePreservesRefs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_HOME", dir)
	path := DeepSeekCredentialsPath()
	if err := os.WriteFile(path, []byte("version: 1\nrefs:\n  EXISTING_KEY: existing-value\nmetadata: keep\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveDeepSeekCredential("NEW_KEY", "new-value"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["version"] != 1 {
		t.Fatalf("version lost: %#v", raw["version"])
	}
	if raw["metadata"] != "keep" {
		t.Fatalf("metadata lost: %#v", raw["metadata"])
	}
	refs, ok := raw["refs"].(map[string]interface{})
	if !ok || refs["EXISTING_KEY"] != "existing-value" || refs["NEW_KEY"] != "new-value" {
		t.Fatalf("refs not preserved: %#v", raw["refs"])
	}
	if _, ok := raw["NEW_KEY"]; ok {
		t.Fatal("new credential duplicated at root")
	}
}

func TestDeepSeekCredentialsWriteLegacyFormat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_HOME", dir)
	path := DeepSeekCredentialsPath()
	if err := os.WriteFile(path, []byte("EXISTING_KEY: existing-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveDeepSeekCredential("NEW_KEY", "new-value"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["NEW_KEY"] != "new-value" || raw["EXISTING_KEY"] != "existing-value" {
		t.Fatalf("legacy credentials not preserved: %#v", raw)
	}
	if _, ok := raw["refs"]; ok {
		t.Fatal("legacy format unexpectedly changed")
	}
}

func TestDshReasoningEffortsFilterUnsupportedVariants(t *testing.T) {
	card := ModelCard{ModelID: "demo", Reasoning: true, Variants: []string{"off", "low", "medium", "high", "xhigh", "max"}, RawCfg: map[string]interface{}{"_dsh_raw": map[string]interface{}{}}}
	raw := dshModelToRaw(card, "openai-compatible")
	efforts, ok := raw["reasoningEfforts"].(map[string]interface{})
	if !ok {
		t.Fatalf("reasoningEfforts=%#v", raw["reasoningEfforts"])
	}
	for _, bad := range []string{"medium", "xhigh"} {
		if _, exists := efforts[bad]; exists {
			t.Fatalf("unsupported effort %q was written", bad)
		}
	}
	for _, good := range []string{"off", "low", "high", "max"} {
		if _, exists := efforts[good]; !exists {
			t.Fatalf("supported effort %q missing", good)
		}
	}
}

func TestEnsureDeepSeekDefaultSupportsMap(t *testing.T) {
	cfg := map[string]interface{}{
		"agent-default-model": map[string]interface{}{"provider": "gone", "model": "gone"},
		"llm-pi-ai":           map[string]interface{}{"providers": map[string]interface{}{"demo": map[string]interface{}{"models": map[string]interface{}{"z-model": map[string]interface{}{}, "a-model": map[string]interface{}{}}}}},
	}
	EnsureDeepSeekDefaultModel(cfg)
	def, _ := cfg["agent-default-model"].(map[string]interface{})
	if def["provider"] != "demo" || def["model"] != "a-model" {
		t.Fatalf("default=%#v", def)
	}
}
