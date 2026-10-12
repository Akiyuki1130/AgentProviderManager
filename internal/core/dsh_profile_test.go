package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/dshprovider"
)

const testDshDesktopPatch = `# Your patch layer for this dsh profile, applied after every bundle layer:
- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        displayName: Acme
        apiKeyEnv: ACME_API_KEY
        api: openai-completions
        baseURL: https://acme.example/v1
        models:
          - id: acme-1
            name: Acme One
            contextWindow: 262144
- id: tools
  config:
    mode: !!js process.env.DSH_TOOLS_MODE
`

func writeDSHProfile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dshTestPayload(id string) ProviderPayload {
	return ProviderPayload{
		"id":               id,
		"name":             "Beta Gateway",
		"kind":             "anthropic",
		"base_url":         "https://beta.example",
		"api_key":          "sk-placeholder",
		"api_key_required": true,
		"cards":            []ModelCard{{ModelID: "beta-1", Name: "Beta One"}},
	}
}

func TestDeepSeekBackendRouting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)

	desktop, err := GetBackend(string(AgentDeepSeek), DeepSeekDesktopProfilePatch())
	if err != nil {
		t.Fatalf("桌面端 profile patch 应拿到 backend：%v", err)
	}
	if _, ok := desktop.(deepseekProfileBackend); !ok {
		t.Fatalf("桌面端 profile patch 应走 profile backend，实际 %T", desktop)
	}
	web, err := GetBackend(string(AgentDeepSeek), DeepSeekWebProfilePatch())
	if err != nil {
		t.Fatalf("web profile patch 应拿到 backend：%v", err)
	}
	if _, ok := web.(deepseekProfileBackend); !ok {
		t.Fatalf("web profile patch 应走 profile backend，实际 %T", web)
	}
	legacy, err := GetBackend(string(AgentDeepSeek), DeepSeekSettingsPath())
	if err != nil {
		t.Fatalf("settings.yaml 应拿到 backend：%v", err)
	}
	if _, ok := legacy.(deepseekLegacyBackend); !ok {
		t.Fatalf("settings.yaml 应走 legacy backend，实际 %T", legacy)
	}
	if DeepSeekDefaultPath() != DeepSeekDesktopProfilePatch() {
		t.Fatalf("没有已存在文件时应默认桌面端 profile：%s", DeepSeekDefaultPath())
	}
}

func TestDeepSeekProfileUpsertWritesPatchAndCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	path := DeepSeekDesktopProfilePatch()
	writeDSHProfile(t, path, testDshDesktopPatch)

	backend, err := GetBackend(string(AgentDeepSeek), path)
	if err != nil {
		t.Fatal(err)
	}
	doc, fingerprint, err := backend.ReadWithFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	store := backend.Store(doc)
	if got := store.List(doc); len(got) != 1 || got[0].ID != "acme" {
		t.Fatalf("List = %#v", got)
	}
	if got := got0(store.List(doc)); got.Kind != "openai-compatible" || got.ModelCount != 1 {
		t.Fatalf("acme 摘要不正确：%#v", got)
	}

	merged, err := store.Upsert(doc, "", dshTestPayload("beta"), SaveOptions{})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := backend.Write(path, merged, fingerprint); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "!!js process.env.DSH_TOOLS_MODE") {
		t.Fatalf("patch 的 !!js 行必须原样保留：%s", text)
	}
	if !strings.Contains(text, "# Your patch layer for this dsh profile") {
		t.Fatalf("patch 注释必须保留：%s", text)
	}
	patch, err := dshprovider.LoadPatch(data)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	if providers["acme"] == nil || providers["beta"] == nil {
		t.Fatalf("providers = %#v", providers)
	}
	beta, _ := providers["beta"].(map[string]interface{})
	if beta["api"] != "anthropic-messages" {
		t.Fatalf("api 协议应写成 anthropic-messages：%#v", beta["api"])
	}
	if beta["apiKeyEnv"] != "BETA_API_KEY" {
		t.Fatalf("应写入 apiKeyEnv 引用：%#v", beta["apiKeyEnv"])
	}
	if _, inline := beta["apiKey"]; inline {
		t.Fatal("明文 API Key 不应留在 patch 里")
	}
	if models, ok := beta["models"].([]interface{}); !ok || len(models) != 1 {
		t.Fatalf("models 应为列表：%#v", beta["models"])
	}

	// 凭据落到 $DSH_HOME/.credentials.yaml，且是 version 1 布局。
	refs, err := dshprovider.LoadCredentialRefs(DeepSeekCredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if refs["BETA_API_KEY"] != "sk-placeholder" {
		t.Fatalf("凭据未写入：%#v", refs)
	}

	// 重新读取：backend 能读回刚写的 provider。
	doc2, err := backend.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	edit, err := backend.Store(doc2).Get(doc2, "beta")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if edit.Name != "Beta Gateway" || edit.BaseURL != "https://beta.example" || edit.Kind != "anthropic" {
		t.Fatalf("读回的 provider 不正确：%#v", edit)
	}
	if len(edit.Cards) != 1 || edit.Cards[0].ModelID != "beta-1" {
		t.Fatalf("读回的模型不正确：%#v", edit.Cards)
	}
}

func got0(list []ProviderSummary) ProviderSummary {
	if len(list) == 0 {
		return ProviderSummary{}
	}
	return list[0]
}

func TestDeepSeekProfileCreatesRowWhenAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	path := DeepSeekWebProfilePatch()
	writeDSHProfile(t, path, "[]")

	backend, err := GetBackend(string(AgentDeepSeek), path)
	if err != nil {
		t.Fatal(err)
	}
	doc, fingerprint, err := backend.ReadWithFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	store := backend.Store(doc)
	if got := store.List(doc); len(got) != 0 {
		t.Fatalf("空 patch 不应有 provider：%#v", got)
	}
	merged, err := store.Upsert(doc, "", dshTestPayload("beta"), SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Write(path, merged, fingerprint); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	patch, err := dshprovider.LoadPatch(data)
	if err != nil {
		t.Fatal(err)
	}
	if !patch.HasLLMPiAIEntry() {
		t.Fatalf("应新增 llm-pi-ai 行：%s", data)
	}
	if _, err := patch.Providers(); err != nil {
		t.Fatalf("新增行应带 providers：%v", err)
	}
}

func TestDeepSeekProfileHomeOverrideRefusesWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	path := DeepSeekDesktopProfilePatch()
	writeDSHProfile(t, path, testDshDesktopPatch)
	writeDSHProfile(t, DeepSeekHomePatch(), `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      home-route:
        baseURL: https://home.example
        api: openai-completions
        models:
          - id: home-1
`)

	backend, err := GetBackend(string(AgentDeepSeek), path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := backend.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	store := backend.Store(doc)
	// 生效视图来自 home 级 patch。
	list := store.List(doc)
	if len(list) != 1 || list[0].ID != "home-route" {
		t.Fatalf("被 home patch 覆盖时应显示 home 的 provider：%#v", list)
	}
	if _, err := store.Upsert(doc, "", dshTestPayload("beta"), SaveOptions{}); err == nil {
		t.Fatal("被 home patch 覆盖时必须拒绝写入 profile patch")
	}
	if _, _, err := store.Delete(doc, "home-route"); err == nil {
		t.Fatal("被 home patch 覆盖时必须拒绝删除")
	}
}

func TestDeepSeekProfileDeleteAndDeleteModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	path := DeepSeekDesktopProfilePatch()
	patchText := `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        displayName: Acme
        baseURL: https://acme.example/v1
        api: openai-completions
        models:
          - id: acme-1
          - id: acme-2
`
	writeDSHProfile(t, path, patchText)

	backend, err := GetBackend(string(AgentDeepSeek), path)
	if err != nil {
		t.Fatal(err)
	}
	doc, fingerprint, err := backend.ReadWithFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	store := backend.Store(doc)
	next, found, err := store.DeleteModel(doc, "acme", "acme-1")
	if err != nil || !found {
		t.Fatalf("DeleteModel = %v, %v", found, err)
	}
	if err := backend.Write(path, next, fingerprint); err != nil {
		t.Fatal(err)
	}
	doc2, fingerprint2, err := backend.ReadWithFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	edit, err := backend.Store(doc2).Get(doc2, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(edit.Cards) != 1 || edit.Cards[0].ModelID != "acme-2" {
		t.Fatalf("删除后剩余模型不正确：%#v", edit.Cards)
	}
	if _, found, err := backend.Store(doc2).DeleteModel(doc2, "acme", "missing-model"); err != nil || found {
		t.Fatalf("不存在的模型应返回 found=false：%v %v", found, err)
	}

	next2, found, err := backend.Store(doc2).Delete(doc2, "acme")
	if err != nil || !found {
		t.Fatalf("Delete = %v, %v", found, err)
	}
	if err := backend.Write(path, next2, fingerprint2); err != nil {
		t.Fatal(err)
	}
	doc3, err := backend.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := backend.Store(doc3).List(doc3); len(got) != 0 {
		t.Fatalf("删除后仍能列出 provider：%#v", got)
	}
}

// TestDeepSeekEditExistingModelKeepsChanges 回归测试：编辑一个已存在的模型
// （ID 不变、字段变化）时，新值必须写回，而不是被旧的原始节点覆盖。
func TestDeepSeekEditExistingModelKeepsChanges(t *testing.T) {
	cases := []struct {
		name string
		path func() string
		text string
	}{
		{
			name: "profile-patch",
			path: DeepSeekDesktopProfilePatch,
			text: `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        displayName: Acme
        apiKeyEnv: ACME_API_KEY
        api: openai-completions
        baseURL: https://acme.example/v1
        models:
          - id: acme-1
            name: Old Name
            contextWindow: 1000
`,
		},
		{
			name: "legacy-settings",
			path: DeepSeekSettingsPath,
			text: `llm-pi-ai:
  providers:
    acme:
      displayName: Acme
      apiKeyEnv: ACME_API_KEY
      api: openai-completions
      baseURL: https://acme.example/v1
      models:
        - id: acme-1
          name: Old Name
          contextWindow: 1000
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("DSH_HOME", home)
			path := tc.path()
			writeDSHProfile(t, path, tc.text)

			backend, err := GetBackend(string(AgentDeepSeek), path)
			if err != nil {
				t.Fatal(err)
			}
			doc, fingerprint, err := backend.ReadWithFingerprint(path)
			if err != nil {
				t.Fatal(err)
			}
			store := backend.Store(doc)
			payload := ProviderPayload{
				"id":               "acme",
				"name":             "Acme",
				"kind":             "openai-compatible",
				"base_url":         "https://acme.example/v1",
				"api_key_required": false,
				"cards":            []ModelCard{{ModelID: "acme-1", Name: "New Name", Context: 9999}},
			}
			merged, err := store.Upsert(doc, "acme", payload, SaveOptions{MergeModels: false})
			if err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			if err := backend.Write(path, merged, fingerprint); err != nil {
				t.Fatalf("Write: %v", err)
			}
			doc2, err := backend.Read(path)
			if err != nil {
				t.Fatal(err)
			}
			edit, err := backend.Store(doc2).Get(doc2, "acme")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if len(edit.Cards) != 1 {
				t.Fatalf("cards = %#v", edit.Cards)
			}
			if edit.Cards[0].Name != "New Name" {
				t.Fatalf("模型改名被丢弃：%#v", edit.Cards[0])
			}
			if got := toFloat(edit.Cards[0].Context); got != 9999 {
				t.Fatalf("模型上下文改动被丢弃：%#v", edit.Cards[0].Context)
			}
		})
	}
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return -1
	}
}

// merge 时不能被“整体重编码”误伤：目标里原本没有推理信息的模型，合并后不应凭空
// 多出 reasoningEfforts: false（DSH 的“显式关闭推理”标记），也不应丢掉目标独有
// 字段或被无谓改写（如把字符串 token 归一化成 int）。
func TestDeepSeekMergeKeepsUnchangedModelNode(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		card  ModelCard
		check func(t *testing.T, mm map[string]interface{})
	}{
		{
			name: "无推理信息不被关闭",
			text: `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        api: openai-completions
        baseURL: https://acme.example/v1
        models:
          - id: acme-1
            contextWindow: 1000
`,
			card: ModelCard{ModelID: "acme-1", Context: 1000},
			check: func(t *testing.T, mm map[string]interface{}) {
				if re, has := mm["reasoningEfforts"]; has {
					t.Fatalf("未改动模型不应被写入 reasoningEfforts：%#v", re)
				}
			},
		},
		{
			name: "目标独有字段保留",
			text: `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        api: openai-completions
        baseURL: https://acme.example/v1
        models:
          - id: acme-1
            contextWindow: 1000
            maxTokens: 300
`,
			card: ModelCard{ModelID: "acme-1", Context: 1000},
			check: func(t *testing.T, mm map[string]interface{}) {
				if _, has := mm["reasoningEfforts"]; has {
					t.Fatalf("未改动模型不应被写入 reasoningEfforts：%#v", mm)
				}
				if ParseTokens(mm["maxTokens"]) == nil || *ParseTokens(mm["maxTokens"]) != 300 {
					t.Fatalf("目标独有字段 maxTokens 丢失：%#v", mm)
				}
			},
		},
		{
			name: "字符串 token 不触发推理关闭",
			text: `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        api: openai-completions
        baseURL: https://acme.example/v1
        models:
          - id: acme-1
            contextWindow: "262144"
            maxTokens: "8192"
`,
			card: ModelCard{ModelID: "acme-1", Context: "262144", Output: "8192"},
			check: func(t *testing.T, mm map[string]interface{}) {
				if _, has := mm["reasoningEfforts"]; has {
					t.Fatalf("语义未变的模型不应被写入 reasoningEfforts：%#v", mm)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("DSH_HOME", home)
			path := DeepSeekDesktopProfilePatch()
			writeDSHProfile(t, path, tc.text)
			backend, err := GetBackend(string(AgentDeepSeek), path)
			if err != nil {
				t.Fatal(err)
			}
			doc, fingerprint, err := backend.ReadWithFingerprint(path)
			if err != nil {
				t.Fatal(err)
			}
			payload := ProviderPayload{
				"id":               "acme",
				"name":             "Acme",
				"kind":             "openai-compatible",
				"base_url":         "https://acme.example/v1",
				"api_key_required": false,
				"cards":            []ModelCard{tc.card},
			}
			merged, err := backend.Store(doc).Upsert(doc, "acme", payload, SaveOptions{MergeModels: true})
			if err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			if err := backend.Write(path, merged, fingerprint); err != nil {
				t.Fatalf("Write: %v", err)
			}
			patch, err := dshprovider.LoadPatch([]byte(mustReadFile(t, path)))
			if err != nil {
				t.Fatal(err)
			}
			providers, err := patch.Providers()
			if err != nil {
				t.Fatal(err)
			}
			acme, _ := providers["acme"].(map[string]interface{})
			models, _ := acme["models"].([]interface{})
			if len(models) != 1 {
				t.Fatalf("models = %#v", acme["models"])
			}
			mm, _ := models[0].(map[string]interface{})
			tc.check(t, mm)
		})
	}
}

// 用户把已开启推理的模型关掉后，必须落成 DSH 的显式关闭标记 reasoningEfforts: false，
// 并且不残留陈旧的单数键 reasoningEffort。
func TestDeepSeekExplicitReasoningOffPersists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	path := DeepSeekDesktopProfilePatch()
	writeDSHProfile(t, path, `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        api: openai-completions
        baseURL: https://acme.example/v1
        models:
          - id: acme-1
            reasoningEfforts: {off: null, high: high, max: max}
            reasoningEffort: high
            contextWindow: 1000
`)
	backend, err := GetBackend(string(AgentDeepSeek), path)
	if err != nil {
		t.Fatal(err)
	}
	doc, fingerprint, err := backend.ReadWithFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	store := backend.Store(doc)
	// 取出现有卡片，模拟 UI 关闭推理（_raw_reasoning_enabled=false）。
	edit, err := store.Get(doc, "acme")
	if err != nil {
		t.Fatal(err)
	}
	card := edit.Cards[0]
	card.Reasoning = false
	off := false
	card.RawReasoningEnabled = &off
	card.Variants = []string{}
	card.DefaultVariant = ""
	payload := ProviderPayload{
		"id": edit.ID, "name": edit.Name, "kind": edit.Kind,
		"base_url": edit.BaseURL, "api_key_required": false,
		"cards": []ModelCard{card},
	}
	merged, err := store.Upsert(doc, "acme", payload, SaveOptions{MergeModels: false})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := backend.Write(path, merged, fingerprint); err != nil {
		t.Fatalf("Write: %v", err)
	}
	patch, err := dshprovider.LoadPatch([]byte(mustReadFile(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	acme, _ := providers["acme"].(map[string]interface{})
	models, _ := acme["models"].([]interface{})
	mm, _ := models[0].(map[string]interface{})
	if mm["reasoningEfforts"] != false {
		t.Fatalf("关闭推理未落成 reasoningEfforts: false：%#v", mm)
	}
	if _, has := mm["reasoningEffort"]; has {
		t.Fatalf("显式关闭后不应残留 reasoningEffort：%#v", mm)
	}
}

// 0.1 legacy settings.yaml 的 llm-pi-ai 段与 0.2 profile patch 共用同一编码路径，
// 关闭推理同样必须落成 reasoningEfforts: false，且合并语义下不能残留单数键、
// 也不能丢掉目标节点独有的字段。
func TestDeepSeekLegacySettingsExplicitReasoningOff(t *testing.T) {
	for _, tc := range []struct {
		name  string
		merge bool
	}{
		{"overwrite", false},
		{"merge", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("DSH_HOME", home)
			path := DeepSeekSettingsPath()
			writeDSHProfile(t, path, `llm-pi-ai:
  providers:
    acme:
      displayName: Acme
      apiKeyEnv: ACME_API_KEY
      baseURL: https://acme.example/v1
      models:
        - id: acme-1
          keepMe: keep-legacy
          reasoningEfforts: {off: null, high: high, max: max}
          reasoningEffort: high
          contextWindow: 1000
`)
			backend, err := GetBackend(string(AgentDeepSeek), path)
			if err != nil {
				t.Fatal(err)
			}
			doc, fingerprint, err := backend.ReadWithFingerprint(path)
			if err != nil {
				t.Fatal(err)
			}
			store := backend.Store(doc)
			edit, err := store.Get(doc, "acme")
			if err != nil {
				t.Fatal(err)
			}
			card := edit.Cards[0]
			card.Reasoning = false
			off := false
			card.RawReasoningEnabled = &off
			card.Variants = []string{}
			card.DefaultVariant = ""
			payload := ProviderPayload{
				"id": edit.ID, "name": edit.Name, "kind": edit.Kind,
				"base_url": edit.BaseURL, "api_key_required": false,
				"cards": []ModelCard{card},
			}
			merged, err := store.Upsert(doc, "acme", payload, SaveOptions{MergeModels: tc.merge})
			if err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			if err := backend.Write(path, merged, fingerprint); err != nil {
				t.Fatalf("Write: %v", err)
			}
			cfg, err := DeepSeekLoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			acme, _ := deepSeekProvidersMap(cfg)["acme"].(map[string]interface{})
			models, _ := acme["models"].([]interface{})
			if len(models) != 1 {
				t.Fatalf("models = %#v", acme["models"])
			}
			mm, _ := models[0].(map[string]interface{})
			if mm["reasoningEfforts"] != false {
				t.Fatalf("legacy settings 关闭推理未落成 reasoningEfforts: false：%#v", mm)
			}
			if _, has := mm["reasoningEffort"]; has {
				t.Fatalf("legacy settings 显式关闭后不应残留 reasoningEffort：%#v", mm)
			}
			if tc.merge && mm["keepMe"] != "keep-legacy" {
				t.Fatalf("合并语义应保留目标独有字段：%#v", mm)
			}
		})
	}
}

func TestDeepSeekLegacySettingsBackendStillWorks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	path := DeepSeekSettingsPath()
	writeDSHProfile(t, path, `llm-pi-ai:
  providers:
    acme:
      displayName: Acme
      apiKeyEnv: ACME_API_KEY
      baseURL: https://acme.example/v1
      models:
        - id: acme-1
`)

	backend, err := GetBackend(string(AgentDeepSeek), path)
	if err != nil {
		t.Fatal(err)
	}
	doc, fingerprint, err := backend.ReadWithFingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	store := backend.Store(doc)
	list := store.List(doc)
	if len(list) != 1 || list[0].ID != "acme" {
		t.Fatalf("legacy settings 应读出 provider：%#v", list)
	}
	merged, err := store.Upsert(doc, "", dshTestPayload("beta"), SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Write(path, merged, fingerprint); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "beta") {
		t.Fatalf("legacy settings 写入失败：%s", data)
	}
	refs, err := dshprovider.LoadCredentialRefs(DeepSeekCredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if refs["BETA_API_KEY"] != "sk-placeholder" {
		t.Fatalf("legacy 写入也应落凭据：%#v", refs)
	}
}

func TestDeepSeekImportPreviewReadsProfilePatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	path := DeepSeekDesktopProfilePatch()
	writeDSHProfile(t, path, testDshDesktopPatch)
	preview := DeepSeekImportPreview(path)
	if preview.Error != "" {
		t.Fatalf("preview error: %s", preview.Error)
	}
	if len(preview.Providers) != 1 || preview.Providers[0].ID != "acme" {
		t.Fatalf("preview providers = %#v", preview.Providers)
	}
	if preview.Providers[0].Kind != "openai-compatible" {
		t.Fatalf("kind = %q", preview.Providers[0].Kind)
	}
}
