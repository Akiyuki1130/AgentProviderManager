package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ompTestModelsYAML 是 store 测试的公共样本：覆盖可写 / auth:none /
// 协议不可建模 / oauth 无 key 四种只读判定。
const ompTestModelsYAML = `# 顶部注释必须保留
providers:
  alpha:
    api: anthropic-messages
    baseUrl: https://alpha.example.com
    apiKey: sk-placeholder
    headers:
      X-Tenant: t1
    transport:
      timeoutMs: 1000
    models:
      - id: alpha-1
        name: Alpha One
        contextWindow: 200000
        maxTokens: 8192
        reasoning: true
        thinking:
          mode: effort
          efforts: [minimal, low, medium, high]
          defaultLevel: high
        input: [text, image]
        supportsTools: true
      - id: alpha-2
  beta:
    api: openai-completions
    baseUrl: https://beta.example.com
    auth: none
    models:
      - id: beta-1
  gamma:
    api: google-generative-ai
    baseUrl: https://gamma.example.com
    apiKey: sk-gamma
    models:
      - id: gamma-1
  delta:
    api: anthropic-messages
    baseUrl: https://delta.example.com
    auth: oauth
    models:
      - id: delta-1
`

// ompTestReadDoc 在临时目录里写一份 models 文件并按真实 backend 读成 Doc。
func ompTestReadDoc(t *testing.T, dir, name, content string) (Doc, string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建临时目录失败：%v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时 models 文件失败：%v", err)
	}
	doc, err := ompBackend{}.Read(path)
	if err != nil {
		t.Fatalf("读取 omp 模型文件失败：%v", err)
	}
	return doc, path
}

// ompTestReadSample 读公共样本，返回 doc 与 models 文件路径。
func ompTestReadSample(t *testing.T) (Doc, string) {
	t.Helper()
	return ompTestReadDoc(t, t.TempDir(), "models.yml", ompTestModelsYAML)
}

// ompTestSaveAndReload 把 Doc 写盘后重新读回，验证落盘内容与再解析结果。
func ompTestSaveAndReload(t *testing.T, path string, doc Doc) (string, Doc) {
	t.Helper()
	if err := (ompBackend{}).Write(path, doc, ""); err != nil {
		t.Fatalf("写入 omp 模型文件失败：%v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取落盘内容失败：%v", err)
	}
	reloaded, err := ompBackend{}.Read(path)
	if err != nil {
		t.Fatalf("重新读取 omp 模型文件失败：%v", err)
	}
	return string(data), reloaded
}

func ompTestSummaryByID(t *testing.T, list []ProviderSummary, id string) ProviderSummary {
	t.Helper()
	for _, item := range list {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("列表里找不到 provider %q：%#v", id, list)
	return ProviderSummary{}
}

func ompTestCardByID(t *testing.T, cards []ModelCard, id string) ModelCard {
	t.Helper()
	for _, card := range cards {
		if card.ModelID == id {
			return card
		}
	}
	t.Fatalf("卡片里找不到模型 %q：%#v", id, cards)
	return ModelCard{}
}

func ompTestStrings(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, ",")
}

func TestOMPStoreListAndGet(t *testing.T) {
	doc, _ := ompTestReadSample(t)
	store := ompStore{}

	list := store.List(doc)
	if len(list) != 4 {
		t.Fatalf("List 长度 = %d，期望 4：%#v", len(list), list)
	}
	if got := []string{list[0].ID, list[1].ID, list[2].ID, list[3].ID}; ompTestStrings(got) != "alpha,beta,delta,gamma" {
		t.Fatalf("List 排序 = %v，期望 alpha,beta,delta,gamma", got)
	}
	alpha := ompTestSummaryByID(t, list, "alpha")
	if alpha.Kind != "anthropic" || alpha.ReadOnly || alpha.BaseURL != "https://alpha.example.com" || !alpha.HasAPIKey || alpha.ModelCount != 2 {
		t.Fatalf("alpha 列表项不符：%#v", alpha)
	}
	if beta := ompTestSummaryByID(t, list, "beta"); beta.Kind != "openai-compatible" || beta.ReadOnly || beta.HasAPIKey {
		t.Fatalf("beta 列表项不符：%#v", beta)
	}
	if gamma := ompTestSummaryByID(t, list, "gamma"); !gamma.ReadOnly || gamma.Kind != "" {
		t.Fatalf("gamma 应只读且 kind 为空：%#v", gamma)
	}
	if delta := ompTestSummaryByID(t, list, "delta"); !delta.ReadOnly {
		t.Fatalf("delta（oauth 无 key）应只读：%#v", delta)
	}

	edit, err := store.Get(doc, "alpha")
	if err != nil {
		t.Fatalf("Get(alpha) 失败：%v", err)
	}
	if edit.ReadOnly || edit.ReadOnlyReason != "" || edit.APIKeyKind != "literal" || edit.APIKey != "sk-placeholder" || edit.RawAPI != "anthropic-messages" {
		t.Fatalf("alpha Get 基础字段不符：%#v", edit)
	}
	if edit.OptionsJSON != `{"headers":{"X-Tenant":"t1"}}` {
		t.Fatalf("OptionsJSON = %q", edit.OptionsJSON)
	}
	if len(edit.Cards) != 2 || edit.Cards[0].ModelID != "alpha-1" {
		t.Fatalf("alpha 卡片不符：%#v", edit.Cards)
	}
	first := ompTestCardByID(t, edit.Cards, "alpha-1")
	if first.Context != 200000 || first.Output != 8192 || !first.Reasoning {
		t.Fatalf("alpha-1 基础字段不符：%#v", first)
	}
	if got := ompTestStrings(first.Variants); got != "off,minimal,low,medium,high" {
		t.Fatalf("alpha-1 Variants = %v", got)
	}
	if got := ompTestStrings(first.SupportedLevels); got != "minimal,low,medium,high" {
		t.Fatalf("alpha-1 SupportedLevels = %v", got)
	}
	if first.DefaultVariant != "high" || first.Vendor == "" {
		t.Fatalf("alpha-1 DefaultVariant/Vendor 不符：%#v", first)
	}
	second := ompTestCardByID(t, edit.Cards, "alpha-2")
	if second.Reasoning || ompTestStrings(second.Variants) != "off" || second.DefaultVariant != "off" {
		t.Fatalf("alpha-2 应未开启 reasoning：%#v", second)
	}

	// 只读 provider 也必须能取到编辑视图（测试连接/刷新模型仍可用），不报错。
	readOnly, err := store.Get(doc, "gamma")
	if err != nil {
		t.Fatalf("Get(gamma) 应成功：%v", err)
	}
	if !readOnly.ReadOnly || readOnly.ReadOnlyReason == "" || readOnly.RawAPI != "google-generative-ai" || readOnly.BaseURL != "https://gamma.example.com" {
		t.Fatalf("gamma Get 只读字段不符：%#v", readOnly)
	}
	if oauth, err := store.Get(doc, "delta"); err != nil || !oauth.ReadOnly || !strings.Contains(oauth.ReadOnlyReason, "OAuth") {
		t.Fatalf("delta Get 应为 OAuth 只读：%#v err=%v", oauth, err)
	}
	if _, err := store.Get(doc, "nope"); err == nil {
		t.Fatal("Get 不存在的 provider 应报错")
	}
}

func TestOMPStoreUpsertRoundTrip(t *testing.T) {
	doc, path := ompTestReadSample(t)
	store := ompStore{}
	payload := ProviderPayload{
		"id":               "alpha",
		"kind":             "openai-compatible",
		"base_url":         "https://alpha2.example.com",
		"api_key":          "sk-new",
		"api_key_required": true,
		"options_json":     `{"headers":{"X-Tenant":"t2"},"authHeader":true}`,
		"cards": []interface{}{
			map[string]interface{}{
				"model_id":        "alpha-1",
				"name":            "Alpha Renamed",
				"context":         300000,
				"output":          16384,
				"reasoning":       true,
				"variants":        []interface{}{"off", "low", "high"},
				"default_variant": "high",
				"capabilities": map[string]interface{}{
					"input_modalities": []interface{}{"text", "image", "audio"},
					"tool_call":        true,
				},
			},
			map[string]interface{}{
				"model_id": "alpha-3",
				"context":  1000,
			},
		},
	}
	if _, err := store.Upsert(doc, "alpha", payload, SaveOptions{MergeModels: true}); err != nil {
		t.Fatalf("Upsert 失败：%v", err)
	}
	text, reloaded := ompTestSaveAndReload(t, path, doc)

	for _, want := range []string{
		"# 顶部注释必须保留",
		"api: openai-completions",
		"baseUrl: https://alpha2.example.com",
		"apiKey: sk-new",
		"X-Tenant: t2",
		"authHeader: true",
		"timeoutMs: 1000",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("落盘内容缺少 %q：\n%s", want, text)
		}
	}
	if strings.Contains(text, "_omp") {
		t.Fatalf("落盘内容出现了内部键：\n%s", text)
	}

	edit, err := store.Get(reloaded, "alpha")
	if err != nil {
		t.Fatalf("Get(alpha) 失败：%v", err)
	}
	if edit.Kind != "openai-compatible" || edit.RawAPI != "openai-completions" || edit.BaseURL != "https://alpha2.example.com" {
		t.Fatalf("写回后 alpha 基础字段不符：%#v", edit)
	}
	if len(edit.Cards) != 3 {
		t.Fatalf("mergeModels 应保留 alpha-2 并新增 alpha-3，实际 %#v", edit.Cards)
	}
	first := ompTestCardByID(t, edit.Cards, "alpha-1")
	ompTestCardByID(t, edit.Cards, "alpha-2")
	if first.Name != "Alpha Renamed" || first.Context != 300000 || first.Output != 16384 {
		t.Fatalf("alpha-1 写回字段不符：%#v", first)
	}
	if got := ompTestStrings(first.Variants); got != "off,low,high" {
		t.Fatalf("alpha-1 Variants = %v，期望 off,low,high", got)
	}
	if first.Capabilities == nil || ompTestStrings(first.Capabilities.InputModalities) != "text,image" {
		t.Fatalf("alpha-1 能力位应裁剪为 text,image：%#v", first.Capabilities)
	}
	if first.Capabilities.ToolCall == nil || !*first.Capabilities.ToolCall {
		t.Fatalf("alpha-1 supportsTools 应写回 true：%#v", first.Capabilities)
	}
	if strings.Contains(text, "audio") {
		t.Fatalf("落盘不应出现 audio 模态：\n%s", text)
	}
}

func TestOMPStoreUpsertAPIKeySemantics(t *testing.T) {
	cases := []struct {
		name       string
		payload    ProviderPayload
		wantKey    bool
		wantAuth   string
		wantErrSub string
	}{
		{
			name: "未触碰且为空=保留已有 key",
			payload: ProviderPayload{
				"id": "alpha", "kind": "anthropic", "base_url": "https://alpha.example.com",
				"api_key": "", "api_key_required": true,
			},
			wantKey: true,
		},
		{
			// 清空 key 会删掉 apiKey 键；但 provider 仍有 models 且不是 auth:none/oauth，
			// omp 会判为非法配置，所以写入必须被拒绝（A 的 Validate 是最终权威）。
			name: "触碰且为空=删除 key 但配置因此非法",
			payload: ProviderPayload{
				"id": "alpha", "kind": "anthropic", "base_url": "https://alpha.example.com",
				"api_key": "", "api_key_required": true, "_api_key_touched": true,
			},
			wantErrSub: "apiKey",
		},
		{
			name: "不需要 key=删除 key 并写 auth none",
			payload: ProviderPayload{
				"id": "alpha", "kind": "anthropic", "base_url": "https://alpha.example.com",
				"api_key": "", "api_key_required": false,
			},
			wantKey:  false,
			wantAuth: "none",
		},
		{
			name: "不需要 key 却填了 key=拒绝",
			payload: ProviderPayload{
				"id": "alpha", "kind": "anthropic", "base_url": "https://alpha.example.com",
				"api_key": "sk-should-not", "api_key_required": false,
			},
			wantErrSub: "不需要 Key",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, path := ompTestReadSample(t)
			store := ompStore{}
			_, err := store.Upsert(doc, "alpha", tc.payload, SaveOptions{MergeModels: true})
			if tc.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("期望错误包含 %q，实际 err=%v", tc.wantErrSub, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Upsert 失败：%v", err)
			}
			text, reloaded := ompTestSaveAndReload(t, path, doc)
			edit, err := store.Get(reloaded, "alpha")
			if err != nil {
				t.Fatalf("Get 失败：%v", err)
			}
			if tc.wantKey && edit.APIKey == "" {
				t.Fatalf("apiKey 应被保留：\n%s", text)
			}
			if !tc.wantKey && edit.APIKey != "" {
				t.Fatalf("apiKey 应被删除：\n%s", text)
			}
			if tc.wantAuth != "" && !strings.Contains(text, "auth: "+tc.wantAuth) {
				t.Fatalf("落盘应包含 auth: %s：\n%s", tc.wantAuth, text)
			}
		})
	}
}

func TestOMPStoreUpsertOptionsJSON(t *testing.T) {
	cases := []struct {
		name       string
		options    string
		wantHeader bool
		wantErrSub string
	}{
		{name: "空串=不修改", options: "", wantHeader: true},
		{name: "显式空 headers=清空", options: `{"headers":{}}`, wantHeader: false},
		{name: "非法键拒绝", options: `{"proxy":"http://x"}`, wantErrSub: "options_json"},
		{name: "headers 值必须字符串", options: `{"headers":{"X-N":1}}`, wantErrSub: "字符串"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, path := ompTestReadSample(t)
			payload := ProviderPayload{
				"id": "alpha", "kind": "anthropic", "base_url": "https://alpha.example.com",
				"api_key": "sk-keep", "api_key_required": true, "options_json": tc.options,
			}
			_, err := ompStore{}.Upsert(doc, "alpha", payload, SaveOptions{MergeModels: true})
			if tc.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Fatalf("期望错误包含 %q，实际 err=%v", tc.wantErrSub, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Upsert 失败：%v", err)
			}
			text, _ := ompTestSaveAndReload(t, path, doc)
			if tc.wantHeader && !strings.Contains(text, "X-Tenant: t1") {
				t.Fatalf("headers 应保持不变：\n%s", text)
			}
			if !tc.wantHeader && strings.Contains(text, "X-Tenant") {
				t.Fatalf("headers 应被清空：\n%s", text)
			}
		})
	}
}

func TestOMPStoreReadOnlyAndMigration(t *testing.T) {
	doc, _ := ompTestReadSample(t)
	store := ompStore{}

	// models.yml 的列表只读态来自单条 provider 的视图判定；这里必须在任何
	// 删除操作之前取样，因为 Delete 会就地改写 doc 持有的文件句柄。
	sampleList := store.List(doc)
	if alpha := ompTestSummaryByID(t, sampleList, "alpha"); alpha.ReadOnly {
		t.Fatalf("models.yml 里可写的 alpha 不应被标只读：%#v", alpha)
	}
	if gamma := ompTestSummaryByID(t, sampleList, "gamma"); !gamma.ReadOnly {
		t.Fatalf("models.yml 里协议不可建模的 gamma 应标只读：%#v", gamma)
	}

	if _, err := store.Upsert(doc, "gamma", ProviderPayload{
		"id": "gamma", "kind": "anthropic", "base_url": "https://gamma.example.com", "api_key_required": false,
	}, SaveOptions{}); err == nil || !strings.Contains(err.Error(), "只读") {
		t.Fatalf("只读 provider 的 Upsert 应被拒绝，err=%v", err)
	}
	if _, _, err := store.DeleteModel(doc, "gamma", "gamma-1"); err == nil || !strings.Contains(err.Error(), "只读") {
		t.Fatalf("只读 provider 的 DeleteModel 应被拒绝，err=%v", err)
	}
	if _, ok, err := store.Delete(doc, "gamma"); err != nil || !ok {
		t.Fatalf("只读 provider 应允许整体删除：ok=%v err=%v", ok, err)
	}

	dir := t.TempDir()
	jsonDoc, jsonPath := ompTestReadDoc(t, dir, "models.json", `{"providers":{"p1":{"api":"anthropic-messages","baseUrl":"https://p1.example.com","models":[{"id":"m1"}]}}}`)
	jsonList := store.List(jsonDoc)
	if len(jsonList) != 1 {
		t.Fatalf("models.json 应能只读列出 provider：%#v", jsonList)
	}
	if !jsonList[0].ReadOnly {
		t.Fatalf("models.json 待迁移态下 List 的每一项都必须标只读：%#v", jsonList)
	}
	if needs, _ := jsonDoc[ompDocNeedsMigration].(bool); !needs {
		t.Fatal("models.json 的 doc 应标记 needs_migration")
	}
	if _, err := store.Upsert(jsonDoc, "p1", ProviderPayload{
		"id": "p1", "kind": "anthropic", "base_url": "https://p1.example.com", "api_key_required": false,
	}, SaveOptions{}); err == nil || !strings.Contains(err.Error(), "models.json") {
		t.Fatalf("models.json 的 Upsert 应被拒绝，err=%v", err)
	}
	if _, _, err := store.Delete(jsonDoc, "p1"); err == nil || !strings.Contains(err.Error(), "models.json") {
		t.Fatalf("models.json 的 Delete 应被拒绝，err=%v", err)
	}
	if err := (ompBackend{}).Write(jsonPath, jsonDoc, ""); err == nil || !strings.Contains(err.Error(), "models.json") {
		t.Fatalf("models.json 的 Write 应被拒绝，err=%v", err)
	}
}

func TestOMPStoreDeleteReferences(t *testing.T) {
	dir := t.TempDir()
	ompTestReadDoc(t, dir, "models.yml", ompTestModelsYAML)
	configYAML := `modelRoles:
  default: alpha/alpha-1
modelPresets:
  fast:
    modelRoles:
      default: gamma/*
    defaultThinkingLevel: low
`
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(configYAML), 0o600); err != nil {
		t.Fatalf("写入 config.yml 失败：%v", err)
	}
	doc, err := ompBackend{}.Read(filepath.Join(dir, "models.yml"))
	if err != nil {
		t.Fatalf("重新读取失败：%v", err)
	}
	store := ompStore{}
	if _, _, err := store.Delete(doc, "alpha"); err == nil || !strings.Contains(err.Error(), "modelRoles") {
		t.Fatalf("被 modelRoles 引用的 provider 删除应被拒绝，err=%v", err)
	}
	if _, _, err := store.DeleteModel(doc, "alpha", "alpha-1"); err == nil || !strings.Contains(err.Error(), "modelRoles") {
		t.Fatalf("被引用的模型删除应被拒绝，err=%v", err)
	}
	if _, _, err := store.DeleteModel(doc, "alpha", "alpha-2"); err != nil {
		t.Fatalf("未被引用的模型应可删除：%v", err)
	}
	if _, ok, err := store.Delete(doc, "beta"); err != nil || !ok {
		t.Fatalf("未被引用的 provider 应可删除：ok=%v err=%v", ok, err)
	}
	if _, _, err := store.Delete(doc, "gamma"); err == nil || !strings.Contains(err.Error(), "modelPresets") {
		t.Fatalf("被 preset 通配引用的 provider 删除应被拒绝，err=%v", err)
	}
}

func TestOMPStoreRenamePreservesUnknownKeys(t *testing.T) {
	doc, path := ompTestReadSample(t)
	payload := ProviderPayload{
		"id": "alpha-renamed", "kind": "anthropic", "base_url": "https://alpha.example.com",
		"api_key": "sk-placeholder", "api_key_required": true,
	}
	if _, err := (ompStore{}).Upsert(doc, "alpha", payload, SaveOptions{MergeModels: true}); err != nil {
		t.Fatalf("重命名失败：%v", err)
	}
	text, reloaded := ompTestSaveAndReload(t, path, doc)
	store := ompStore{}
	list := store.List(reloaded)
	ids := make([]string, 0, len(list))
	for _, item := range list {
		ids = append(ids, item.ID)
	}
	if ompTestStrings(ids) != "alpha-renamed,beta,delta,gamma" {
		t.Fatalf("重命名后 providers = %v", ids)
	}
	if !strings.Contains(text, "timeoutMs: 1000") {
		t.Fatalf("重命名应保留未知键：\n%s", text)
	}
	edit, err := store.Get(reloaded, "alpha-renamed")
	if err != nil {
		t.Fatalf("Get(alpha-renamed) 失败：%v", err)
	}
	if len(edit.Cards) != 2 || edit.Cards[0].ModelID != "alpha-1" {
		t.Fatalf("重命名应保留模型：%#v", edit.Cards)
	}
	if _, err := store.Get(reloaded, "alpha"); err == nil {
		t.Fatal("旧 ID 应已被删除")
	}
}

func TestOMPStoreCreateProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent", "models.yml")
	doc, err := ompBackend{}.Read(path)
	if err != nil {
		t.Fatalf("缺失文件应返回空文档：%v", err)
	}
	store := ompStore{}
	if len(store.List(doc)) != 0 {
		t.Fatalf("空文档不应有 provider：%#v", store.List(doc))
	}
	payload := ProviderPayload{
		"id": "new1", "kind": "anthropic", "base_url": "https://new.example.com",
		"api_key": "sk-new", "api_key_required": true,
		"cards": []interface{}{
			map[string]interface{}{"model_id": "new-model", "context": 1000},
		},
	}
	if _, err := store.Upsert(doc, "", payload, SaveOptions{}); err != nil {
		t.Fatalf("新建 provider 失败：%v", err)
	}
	text, reloaded := ompTestSaveAndReload(t, path, doc)
	for _, want := range []string{"new1:", "api: anthropic-messages", "baseUrl: https://new.example.com", "apiKey: sk-new", "id: new-model", "- text"} {
		if !strings.Contains(text, want) {
			t.Fatalf("新建结果缺少 %q：\n%s", want, text)
		}
	}
	if strings.Contains(text, "_omp") {
		t.Fatalf("落盘内容出现了内部键：\n%s", text)
	}
	edit, err := store.Get(reloaded, "new1")
	if err != nil {
		t.Fatalf("Get(new1) 失败：%v", err)
	}
	if len(edit.Cards) != 1 || edit.Cards[0].ModelID != "new-model" {
		t.Fatalf("新建模型不符：%#v", edit.Cards)
	}
	if edit.Cards[0].Capabilities == nil || ompTestStrings(edit.Cards[0].Capabilities.InputModalities) != "text" {
		t.Fatalf("新建模型默认 input 应为 text：%#v", edit.Cards[0].Capabilities)
	}
}

func TestOMPStoreRejectsInvalidProviderID(t *testing.T) {
	doc, _ := ompTestReadSample(t)
	store := ompStore{}
	for _, id := range []string{"bad/id", "bad id", "bad+id"} {
		if _, err := store.Upsert(doc, "", ProviderPayload{
			"id": id, "kind": "anthropic", "base_url": "https://x.example.com", "api_key_required": false,
		}, SaveOptions{}); err == nil || !strings.Contains(err.Error(), "非法") {
			t.Fatalf("ID %q 应被拒绝，err=%v", id, err)
		}
	}
}
