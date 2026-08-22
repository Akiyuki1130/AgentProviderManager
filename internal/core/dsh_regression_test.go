package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestDshReasoningEffortsPreservedFromZCodeCard(t *testing.T) {
	model := map[string]interface{}{
		"reasoning": map[string]interface{}{
			"enabled":        true,
			"variants":       []interface{}{"off", "low", "medium", "high", "xhigh", "max"},
			"defaultVariant": "high",
		},
	}
	card := CfgToCard("demo", model)
	if !card.Reasoning || !reflect.DeepEqual(card.Variants, []string{"off", "low", "medium", "high", "xhigh", "max"}) {
		t.Fatalf("card reasoning state lost: %+v", card)
	}
	raw := dshModelToRaw(card, "openai-compatible")
	efforts, ok := raw["reasoningEfforts"].(map[string]interface{})
	if !ok {
		t.Fatalf("reasoningEfforts missing for non-native source: %#v", raw)
	}
	for _, good := range []string{"off", "low", "high", "max"} {
		if _, exists := efforts[good]; !exists {
			t.Fatalf("supported effort %q missing", efforts)
		}
	}
	for _, bad := range []string{"medium", "xhigh"} {
		if _, exists := efforts[bad]; exists {
			t.Fatalf("unsupported effort %q was written", bad)
		}
	}
}

func TestDshReasoningEffortsPreserveEnabledAndSingleVariant(t *testing.T) {
	enabled := true
	raw := dshModelToRaw(ModelCard{
		ModelID:             "demo",
		Reasoning:           true,
		RawReasoningEnabled: &enabled,
		Variants:            []string{"high"},
		RawCfg:              map[string]interface{}{"_opencode_raw": map[string]interface{}{}},
	}, "openai-compatible")
	efforts, ok := raw["reasoningEfforts"].(map[string]interface{})
	if !ok || efforts["high"] != "high" {
		t.Fatalf("single supported effort was not preserved: %#v", raw["reasoningEfforts"])
	}

	disabled := false
	raw = dshModelToRaw(ModelCard{
		ModelID:             "demo",
		Reasoning:           true,
		RawReasoningEnabled: &disabled,
		Variants:            []string{"off", "high", "max"},
		RawCfg:              map[string]interface{}{},
	}, "openai-compatible")
	if raw["reasoningEfforts"] != false {
		t.Fatalf("disabled reasoning state was not preserved: %#v", raw["reasoningEfforts"])
	}
}

func TestZCodeToDeepSeekPreservesReasoningState(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "zcode.json")
	targetPath := filepath.Join(dir, "dsh.json")
	source := map[string]interface{}{
		"provider": map[string]interface{}{
			"demo": map[string]interface{}{
				"name": "Demo", "kind": "openai-compatible", "source": "custom",
				"options": map[string]interface{}{"baseURL": "https://api.example.com/v1"},
				"models": map[string]interface{}{
					"reasoner": map[string]interface{}{
						"reasoning": map[string]interface{}{
							"enabled": true, "variants": []interface{}{"off", "low", "medium", "high", "xhigh", "max"}, "defaultVariant": "high",
						},
						"limit": map[string]interface{}{"context": 128000, "output": 4096},
					},
				},
			},
		},
	}
	target := map[string]interface{}{"llm-pi-ai": map[string]interface{}{"providers": map[string]interface{}{}}}
	for path, value := range map[string]interface{}{sourcePath: source, targetPath: target} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := MigrateExecuteAtPaths("zcode", "deepseek", sourcePath, targetPath, []string{"demo"}, "overwrite")
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != true {
		t.Fatalf("migration failed: %#v", result)
	}
	out, err := DeepSeekLoadConfig(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	providers, _ := DeepSeekProvidersMap(out)["demo"].(map[string]interface{})
	models, _ := providers["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("models=%#v", providers["models"])
	}
	model, _ := models[0].(map[string]interface{})
	efforts, _ := model["reasoningEfforts"].(map[string]interface{})
	if len(efforts) != 4 || efforts["low"] != "low" || efforts["high"] != "high" || efforts["max"] != "max" {
		t.Fatalf("reasoning efforts were not preserved: %#v", model["reasoningEfforts"])
	}
	if _, ok := efforts["medium"]; ok {
		t.Fatal("unsupported medium effort was written")
	}
	if _, ok := efforts["xhigh"]; ok {
		t.Fatal("unsupported xhigh effort was written")
	}
	if model["reasoningEffort"] != "high" {
		t.Fatalf("default reasoning effort was not preserved: %#v", model["reasoningEffort"])
	}
	if ParseTokens(model["contextWindow"]) == nil || *ParseTokens(model["contextWindow"]) != 128000 || ParseTokens(model["maxTokens"]) == nil || *ParseTokens(model["maxTokens"]) != 4096 {
		t.Fatalf("limits were not preserved: %#v", model)
	}
}

func TestDeepSeekReasoningConvertsToOtherAgents(t *testing.T) {
	native := map[string]interface{}{
		"api": "openai-completions", "baseURL": "https://api.example.com/v1",
		"models": []interface{}{map[string]interface{}{
			"id":               "reasoner",
			"reasoningEfforts": map[string]interface{}{"off": nil, "low": "low", "high": "high", "max": "max"},
			"reasoningEffort":  "low",
		}},
	}
	_, internal, err := ConvertDeepSeekProvider("demo", native, nil)
	if err != nil {
		t.Fatal(err)
	}
	models, _ := internal["models"].(map[string]interface{})
	model, _ := models["reasoner"].(map[string]interface{})
	reasoning, _ := model["reasoning"].(map[string]interface{})
	if reasoning["enabled"] != true || reasoning["defaultVariant"] != "low" {
		t.Fatalf("reasoning state/default changed during DSH normalization: %#v", model)
	}
	variants, _ := reasoning["variants"].([]string)
	if !reflect.DeepEqual(variants, []string{"off", "low", "high", "max"}) {
		t.Fatalf("variants changed during DSH normalization: %#v", variants)
	}

	opencode := OpenCodeProviderFromCfg(internal)
	nativeModels, _ := opencode["models"].(map[string]interface{})
	nativeModel, _ := nativeModels["reasoner"].(map[string]interface{})
	if nativeModel["reasoning"] != true {
		t.Fatalf("reasoning disabled during OpenCode conversion: %#v", nativeModel)
	}
	options, _ := nativeModel["options"].(map[string]interface{})
	if options["reasoningEffort"] != "low" {
		t.Fatalf("default reasoning effort was not preserved for OpenCode: %#v", options)
	}
	openVariants, _ := nativeModel["variants"].(map[string]interface{})
	for _, name := range []string{"none", "low", "high", "max"} {
		if _, ok := openVariants[name]; !ok {
			t.Fatalf("OpenCode effort %q missing: %#v", name, openVariants)
		}
	}
}

func TestDeepSeekToZCodePreservesReasoningState(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "dsh.json")
	targetPath := filepath.Join(dir, "zcode.json")
	source := map[string]interface{}{
		"llm-pi-ai": map[string]interface{}{"providers": map[string]interface{}{
			"demo": map[string]interface{}{
				"baseURL": "https://api.example.com/v1",
				"models": []interface{}{map[string]interface{}{
					"id":               "reasoner",
					"reasoningEfforts": map[string]interface{}{"off": nil, "low": "low", "high": "high", "max": "max"},
					"reasoningEffort":  "low",
				}},
			},
		}},
	}
	target := map[string]interface{}{"provider": map[string]interface{}{}}
	for path, value := range map[string]interface{}{sourcePath: source, targetPath: target} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := MigrateExecuteAtPaths("deepseek", "zcode", sourcePath, targetPath, []string{"demo"}, "overwrite")
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != true {
		t.Fatalf("migration failed: %#v", result)
	}
	out, err := LoadConfig(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	providers, _ := out["provider"].(map[string]interface{})
	provider, _ := providers["demo"].(map[string]interface{})
	models, _ := provider["models"].(map[string]interface{})
	model, _ := models["reasoner"].(map[string]interface{})
	reasoning, _ := model["reasoning"].(map[string]interface{})
	if reasoning["enabled"] != true {
		t.Fatalf("reasoning disabled during reverse migration: %#v", model)
	}
	variants, _ := reasoning["variants"].([]interface{})
	if len(variants) != 4 || variants[0] != "off" || variants[1] != "low" || variants[2] != "high" || variants[3] != "max" {
		t.Fatalf("reverse migration variants changed: %#v", variants)
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
