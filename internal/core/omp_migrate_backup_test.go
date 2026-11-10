package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectOMPBackupProviders(t *testing.T) {
	base := ProviderEdit{
		ID:             "alpha",
		Name:           "Alpha 显示名",
		Kind:           "anthropic",
		BaseURL:        "https://alpha.example.invalid/v1",
		APIKey:         "PLACEHOLDER_KEY",
		APIKeyRequired: true,
		OptionsJSON:    `{"headers":{"Authorization":"Bearer x","X-Api-Key":"y","X-Trace":"trace-1"},"authHeader":true,"junk":1}`,
		Cards: []ModelCard{{
			ModelID: "m1",
			Headers: map[string]string{"Authorization": "Bearer z"},
			Options: map[string]interface{}{"compat": map[string]interface{}{"x": 1}},
			RawCfg:  map[string]interface{}{"apiKey": "LEAK"},
		}},
		RawProvider: map[string]interface{}{"apiKey": "LEAK", "compat": map[string]interface{}{}},
	}
	readOnly := base
	readOnly.ID = "ro"
	readOnly.ReadOnly = true
	readOnly.ReadOnlyReason = "协议不可建模"

	badKind := base
	badKind.ID = "bad"
	badKind.Kind = "google"
	badKind.RawAPI = "google-generative-ai"

	noBase := base
	noBase.ID = "nobase"
	noBase.BaseURL = ""

	envKey := base
	envKey.ID = "env"
	envKey.APIKey = "APM_OMP_TEST_EXPORT_KEY"
	envKey.APIKeyKind = "env"

	cmdKey := base
	cmdKey.ID = "cmd"
	cmdKey.APIKey = "!cat placeholder.key"
	cmdKey.APIKeyKind = ""

	t.Setenv("APM_OMP_TEST_EXPORT_KEY", "resolved-value")

	out, skipped := ProjectOMPBackupProviders([]ProviderEdit{base, readOnly, badKind, noBase, envKey, cmdKey})
	if len(out) != 3 {
		t.Fatalf("导出条数 = %d（%#v），期望 3", len(out), out)
	}
	if len(skipped) != 3 {
		t.Fatalf("skipped = %#v，期望 3 条", skipped)
	}
	reasons := map[string]string{}
	for _, s := range skipped {
		reasons[s.ID] = s.Reason
	}
	if reasons["ro"] == "" || reasons["bad"] == "" || reasons["nobase"] == "" {
		t.Fatalf("skipped 必须带原因：%#v", skipped)
	}

	got := out[0]
	if got.Name != "" || got.RawProvider != nil || got.APIKeyKind != "" || got.ReadOnly {
		t.Fatalf("投影只保留 id/base_url/kind/api_key/cards：%#v", got)
	}
	if got.APIKey != "PLACEHOLDER_KEY" || got.BaseURL != base.BaseURL || got.Kind != "anthropic" {
		t.Fatalf("基础字段投影不符：%#v", got)
	}
	if strings.Contains(got.OptionsJSON, "Authorization") || strings.Contains(got.OptionsJSON, "X-Api-Key") {
		t.Fatalf("options_json 必须剥离敏感头：%s", got.OptionsJSON)
	}
	if !strings.Contains(got.OptionsJSON, "X-Trace") || !strings.Contains(got.OptionsJSON, "authHeader") {
		t.Fatalf("options_json 应保留非敏感 headers 与 authHeader：%s", got.OptionsJSON)
	}
	if strings.Contains(got.OptionsJSON, "junk") {
		t.Fatalf("options_json 只允许 headers / authHeader：%s", got.OptionsJSON)
	}
	if len(got.Cards) != 1 {
		t.Fatalf("cards 数量 = %d", len(got.Cards))
	}
	if got.Cards[0].RawCfg != nil || got.Cards[0].Options != nil {
		t.Fatalf("卡片私有载体必须清空：%#v", got.Cards[0])
	}
	if base.Cards[0].RawCfg == nil || base.RawProvider == nil {
		t.Fatalf("投影不得改动入参")
	}

	byID := map[string]ProviderEdit{}
	for _, e := range out {
		byID[e.ID] = e
	}
	if byID["env"].APIKey != "" || byID["cmd"].APIKey != "" {
		t.Fatalf("环境变量名与 !command 不得当密钥导出：env=%q cmd=%q", byID["env"].APIKey, byID["cmd"].APIKey)
	}
	if byID["alpha"].Cards == nil || byID["env"].Cards == nil {
		t.Fatalf("cards 不得为 nil")
	}
}

func TestProjectOMPMigrateEdit(t *testing.T) {
	edit := &ProviderEdit{
		ID:      "alpha",
		Name:    "Alpha 显示名",
		Kind:    "openai-compatible",
		BaseURL: "https://alpha.example.invalid/v1",
		APIKey:  "PLACEHOLDER_KEY",
		Cards: []ModelCard{{
			ModelID: "m1",
			Headers: map[string]string{"Authorization": "Bearer z"},
			Options: map[string]interface{}{"compat": map[string]interface{}{"x": 1}},
			RawCfg:  map[string]interface{}{"api": "openai-completions"},
		}},
		RawProvider: map[string]interface{}{"apiKey": "LEAK", "compat": map[string]interface{}{}},
	}
	got, err := projectOMPMigrateEdit(edit)
	if err != nil {
		t.Fatalf("可迁移 provider 不应报错：%v", err)
	}
	if got.Name != "" || got.RawProvider != nil || got.OptionsJSON != "" {
		t.Fatalf("迁移载荷只贡献 id/kind/base_url/api_key/cards：%#v", got)
	}
	if got.APIKey != "PLACEHOLDER_KEY" || len(got.Cards) != 1 {
		t.Fatalf("基础字段投影不符：%#v", got)
	}
	if got.Cards[0].Headers != nil || got.Cards[0].Options != nil || got.Cards[0].RawCfg != nil {
		t.Fatalf("模型私有载体必须清空：%#v", got.Cards[0])
	}

	cases := []struct {
		name string
		edit *ProviderEdit
		want string
	}{
		{name: "只读", edit: &ProviderEdit{ID: "p", ReadOnly: true, ReadOnlyReason: "缺 baseUrl"}, want: "缺 baseUrl"},
		{name: "协议不可建模", edit: &ProviderEdit{ID: "p", Kind: "", RawAPI: "bedrock"}, want: "bedrock"},
		{name: "缺 baseUrl", edit: &ProviderEdit{ID: "p", Kind: "anthropic"}, want: "baseUrl"},
		{name: "nil edit", edit: nil, want: "不存在"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := projectOMPMigrateEdit(tc.edit); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v，期望包含 %q", err, tc.want)
			}
		})
	}
}

func TestOMPMigrateTargetPath(t *testing.T) {
	root := ompConfigSandbox(t)

	// 只有 models.json：不可写，resolveMigratePath 返回空串。
	writeOMPTestFile(t, filepath.Join(root, "agent", "models.json"), `{"providers":{}}`)
	if got := resolveMigratePath(string(AgentOMP), ""); got != "" {
		t.Fatalf("json-only 目标路径 = %q，期望空串（不可写）", got)
	}
	if _, err := ompMigrateTargetPath(""); err == nil || !strings.Contains(err.Error(), "models.json") {
		t.Fatalf("json-only 目标应报不可写：%v", err)
	}

	// 有 models.yml：解析到 models.yml。
	yml := filepath.Join(root, "agent", "models.yml")
	writeOMPTestFile(t, yml, ompModelsFixture)
	if got := resolveMigratePath(string(AgentOMP), ""); !strings.EqualFold(filepath.Clean(got), filepath.Clean(yml)) {
		t.Fatalf("目标路径 = %q，期望 %q", got, yml)
	}
	// 显式路径优先。
	explicit := filepath.Join(root, "custom", "models.yaml")
	if got, err := ompMigrateTargetPath(explicit); err != nil || got != explicit {
		t.Fatalf("显式路径应原样使用：got=%q err=%v", got, err)
	}
}

func TestMigratePreviewOMPWarnings(t *testing.T) {
	src := ompConfigSandbox(t)
	writeOMPTestFile(t, filepath.Join(src, "agent", "models.yml"), ompModelsFixture)

	preview := MigratePreviewAtPaths(string(AgentOMP), string(AgentDeepSeek), "", filepath.Join(t.TempDir(), "settings.yaml"))
	if len(preview.Source.Warnings) == 0 || len(preview.Source.DroppedFields) == 0 {
		t.Fatalf("omp 源预览必须带警告与丢弃清单：%#v", preview.Source)
	}
	if preview.Source.Format != "models.yml" {
		t.Fatalf("MigrateSide.format = %q，期望 models.yml", preview.Source.Format)
	}
}

// 确保 os 包在该文件中至少被引用一次，避免不同构建标签下 import 失效。
var _ = os.Getenv
