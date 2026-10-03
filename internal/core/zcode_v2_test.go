package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/zcodeprovider"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入测试文件 %s：%v", path, err)
	}
}

// canonicalZCodeV2 返回一份包含给定 providerId 的合法新版配置字节。
func canonicalZCodeV2(t *testing.T, providerIDs ...string) []byte {
	t.Helper()
	cfg := zcodeprovider.NewConfig()
	for _, id := range providerIDs {
		cfg.UpsertProviderRule(zcodeprovider.NewProviderRule(id, id, zcodeprovider.GroupStandardPersonal))
	}
	cfg.SetProviderOrder(providerIDs)
	data, err := zcodeprovider.Encode(cfg)
	if err != nil {
		t.Fatalf("构造 v2 配置失败：%v", err)
	}
	return data
}

func TestDetectZCodeFormatAt(t *testing.T) {
	dir := t.TempDir()

	emptyPath := filepath.Join(dir, "empty.json")
	writeTestFile(t, emptyPath, "  \n\t")
	if got := DetectZCodeFormatAt(emptyPath); got != zcodeprovider.FormatUnknown {
		t.Errorf("空文件应判定为 unknown，得到 %v", got)
	}

	missingPath := filepath.Join(dir, "missing.json")
	if got := DetectZCodeFormatAt(missingPath); got != zcodeprovider.FormatUnknown {
		t.Errorf("不存在文件应判定为 unknown，得到 %v", got)
	}
	if got := DetectZCodeFormatAt(""); got != zcodeprovider.FormatUnknown {
		t.Errorf("空路径应判定为 unknown，得到 %v", got)
	}

	legacyPath := filepath.Join(dir, "config.json")
	writeTestFile(t, legacyPath, `{"provider":{"a":{"name":"A"}}}`)
	if got := DetectZCodeFormatAt(legacyPath); got != zcodeprovider.FormatLegacy {
		t.Errorf("旧版文件应判定为 legacy，得到 %v", got)
	}

	v2Path := filepath.Join(dir, "provider_config.json")
	if err := os.WriteFile(v2Path, canonicalZCodeV2(t, "a"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectZCodeFormatAt(v2Path); got != zcodeprovider.FormatV2 {
		t.Errorf("新版文件应判定为 v2，得到 %v", got)
	}

	if name := ZCodeFormatName(zcodeprovider.FormatV2); name != "v2" {
		t.Errorf("FormatV2 的名称应为 v2，得到 %q", name)
	}
	if name := ZCodeFormatName(zcodeprovider.FormatLegacy); name != "legacy" {
		t.Errorf("FormatLegacy 的名称应为 legacy，得到 %q", name)
	}
	if name := ZCodeFormatName(zcodeprovider.FormatUnknown); name != "unknown" {
		t.Errorf("FormatUnknown 的名称应为 unknown，得到 %q", name)
	}
}

func TestPreviewLegacyImport(t *testing.T) {
	legacyDoc := Doc{
		"provider": map[string]interface{}{
			"pending": map[string]interface{}{
				"name": "Pending",
				"kind": "openai-compatible",
				"options": map[string]interface{}{
					"baseURL":         "https://pending.example.com/v1",
					"apiKey":          "sk-pending",
					"apiKeyRequired":  true,
					"customTransport": "x",
				},
				"models": map[string]interface{}{
					"m1": map[string]interface{}{"limit": map[string]interface{}{"context": 128000}},
				},
			},
			"done": map[string]interface{}{
				"name":    "Done",
				"kind":    "anthropic",
				"options": map[string]interface{}{"baseURL": "https://done.example.com/v1"},
			},
		},
	}
	v2Doc := canonicalV2DocForTest(t, "done")

	preview, err := PreviewLegacyImport(legacyDoc, v2Doc)
	if err != nil {
		t.Fatalf("PreviewLegacyImport：%v", err)
	}
	if len(preview.Providers) != 1 || preview.Providers[0].ID != "pending" {
		t.Fatalf("待导入列表应只有 pending，得到 %+v", preview.Providers)
	}
	got := preview.Providers[0]
	if got.Name != "Pending" || got.Kind != "openai-compatible" || got.BaseURL != "https://pending.example.com/v1" {
		t.Errorf("pending 摘要不正确：%+v", got)
	}
	if !got.HasAPIKey || got.ModelCount != 1 {
		t.Errorf("pending 摘要的 api key / 模型数不正确：%+v", got)
	}
	dropped := preview.Dropped["pending"]
	wantDropped := []string{"options.apiKeyRequired", "options.customTransport"}
	if strings.Join(dropped, ",") != strings.Join(wantDropped, ",") {
		t.Errorf("pending 的丢失字段应为 %v，得到 %v", wantDropped, dropped)
	}
	if _, ok := preview.Dropped["done"]; ok {
		t.Error("已存在的 provider 不应出现在 dropped 中")
	}

	// 没有新版文档（nil）时，旧版全部视为待导入。
	all, err := PreviewLegacyImport(legacyDoc, nil)
	if err != nil {
		t.Fatalf("PreviewLegacyImport(nil v2)：%v", err)
	}
	if len(all.Providers) != 2 {
		t.Fatalf("nil 新版文档时应有两个待导入项，得到 %+v", all.Providers)
	}
	// 入参不被修改。
	if _, ok := legacyDoc["config"]; ok {
		t.Error("PreviewLegacyImport 不应修改 legacy 文档")
	}

	bad := Doc{"provider": "not-an-object"}
	if _, err := PreviewLegacyImport(bad, nil); err == nil {
		t.Error("provider 字段类型错误时应返回错误")
	}
}

func TestApplyLegacyImport(t *testing.T) {
	dir := t.TempDir()
	v2Path := filepath.Join(dir, "provider_config.json")
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", v2Path)

	v2Before := canonicalZCodeV2(t, "existing")
	if err := os.WriteFile(v2Path, v2Before, 0644); err != nil {
		t.Fatal(err)
	}

	legacyPath := filepath.Join(dir, "config.json")
	writeTestFile(t, legacyPath, `{
		"provider": {
			"newp": {
				"name": "New P",
				"kind": "openai-compatible",
				"options": {"baseURL": "https://new.example.com/v1", "apiKey": "sk-new"},
				"models": {
					"m1": {
						"limit": {"context": 200000, "output": 8000},
						"reasoning": {"variants": ["off", "high"]},
						"modalities": {"input": ["image"]}
					}
				}
			},
			"existing": {"name": "Existing", "kind": "anthropic", "options": {"baseURL": "https://existing.example.com/v1"}}
		}
	}`)
	legacyBefore, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	legacyDoc, err := LoadConfig(legacyPath)
	if err != nil {
		t.Fatalf("读取旧版文件：%v", err)
	}
	v2Doc, err := LoadConfig(v2Path)
	if err != nil {
		t.Fatalf("读取新版文件：%v", err)
	}

	newDoc, report, err := ApplyLegacyImport(v2Doc, legacyDoc, []string{"newp", "existing", "ghost"}, v2Path)
	if err != nil {
		t.Fatalf("ApplyLegacyImport：%v", err)
	}
	if strings.Join(report.Imported, ",") != "newp" {
		t.Errorf("imported 应为 [newp]，得到 %v", report.Imported)
	}
	if strings.Join(report.Skipped, ",") != "existing,ghost" {
		t.Errorf("skipped 应为 [existing ghost]，得到 %v", report.Skipped)
	}

	// 返回值里已经含导入项，且 providerOrder 追加在末尾。
	cfg := zcodeprovider.NewConfigFromDoc(newDoc)
	if cfg.ProviderRule("newp") == nil {
		t.Error("返回文档里应含 newp 的 providerRule")
	}
	if order := strings.Join(cfg.ProviderOrder(), ","); order != "existing,newp" {
		t.Errorf("providerOrder 应为 existing,newp，得到 %q", order)
	}

	// 旧版文件一个字节都没变。
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacyBefore, legacyAfter) {
		t.Error("ApplyLegacyImport 不应修改旧版 config.json")
	}

	// 落盘的是 canonical 新版文件：严格解码通过、含导入项、含模型规则。
	written, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := zcodeprovider.Decode(written)
	if err != nil {
		t.Fatalf("写出的新版文件未通过严格解码：%v", err)
	}
	if decoded.ProviderRule("newp") == nil {
		t.Error("新版文件里应含导入的 newp")
	}
	if decoded.ProviderModelRule("newp", "m1") == nil {
		t.Error("新版文件里应含 newp 的模型规则 m1")
	}
	if order := strings.Join(decoded.ProviderOrder(), ","); order != "existing,newp" {
		t.Errorf("新版文件的 providerOrder 应为 existing,newp，得到 %q", order)
	}
	if !bytes.HasSuffix(written, []byte("\n")) {
		t.Error("canonical 编码应以换行结尾")
	}
	if written[len(written)-2] == '\n' {
		t.Error("canonical 编码不应有两个连续换行")
	}

	// 导入前的备份与原文一致。
	if report.Backup == "" {
		t.Fatal("导入前应产生备份")
	}
	backupData, err := os.ReadFile(report.Backup)
	if err != nil {
		t.Fatalf("读取备份：%v", err)
	}
	if !bytes.Equal(backupData, v2Before) {
		t.Error("备份内容应等于导入前的新版文件")
	}

	// 全部都是 skipped 时不备份、不写盘。
	_, again, err := ApplyLegacyImport(v2Doc, legacyDoc, []string{"existing"}, v2Path)
	if err != nil {
		t.Fatalf("ApplyLegacyImport（全部跳过）：%v", err)
	}
	if again.Backup != "" || len(again.Imported) != 0 {
		t.Errorf("没有导入项时不应备份或导入：%+v", again)
	}
}

// TestApplyLegacyImportWritesGivenTarget 锁住“写到实际目标”的行为：
// 目标手工指到别的 v2 文件时，导入必须落在该文件上，canonical 路径不能被顺手改写。
func TestApplyLegacyImportWritesGivenTarget(t *testing.T) {
	dir := t.TempDir()
	canonicalPath := filepath.Join(dir, "canonical", "provider_config.json")
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", canonicalPath)
	if err := os.MkdirAll(filepath.Dir(canonicalPath), 0755); err != nil {
		t.Fatal(err)
	}
	canonicalBefore := canonicalZCodeV2(t, "keep")
	if err := os.WriteFile(canonicalPath, canonicalBefore, 0644); err != nil {
		t.Fatal(err)
	}

	customPath := filepath.Join(dir, "custom_provider_config.json")
	if err := os.WriteFile(customPath, canonicalZCodeV2(t, "existing"), 0644); err != nil {
		t.Fatal(err)
	}

	legacyDoc := Doc{
		"provider": map[string]interface{}{
			"newp": map[string]interface{}{
				"name":    "New P",
				"kind":    "openai-compatible",
				"options": map[string]interface{}{"baseURL": "https://new.example.com/v1", "apiKey": "sk-placeholder"},
			},
		},
	}
	v2Doc, err := LoadConfig(customPath)
	if err != nil {
		t.Fatalf("读取自定义目标文件：%v", err)
	}
	_, report, err := ApplyLegacyImport(v2Doc, legacyDoc, []string{"newp"}, customPath)
	if err != nil {
		t.Fatalf("ApplyLegacyImport：%v", err)
	}
	if report.Backup == "" {
		t.Error("导入前应在目标文件旁产生备份")
	}
	written, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("读取目标文件：%v", err)
	}
	decoded, err := zcodeprovider.Decode(written)
	if err != nil {
		t.Fatalf("目标文件未通过严格解码：%v", err)
	}
	if decoded.ProviderRule("newp") == nil {
		t.Error("导入结果应写入传入的目标文件")
	}
	after, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("读取 canonical 路径：%v", err)
	}
	if !bytes.Equal(after, canonicalBefore) {
		t.Error("目标不是 canonical 路径时，canonical 文件不应被改写")
	}
}

// TestLegacyImportSkipsManagedProviders 锁住“内建不进待导入列表”：
// builtin: / account: 前缀的供应商由 ZCode 自带数据或账号体系管理，
// 预览里不应出现，执行时即使用户手工选中也只能记为 skipped。
func TestLegacyImportSkipsManagedProviders(t *testing.T) {
	legacyDoc := Doc{
		"provider": map[string]interface{}{
			"mine": map[string]interface{}{
				"name":    "Mine",
				"kind":    "openai-compatible",
				"options": map[string]interface{}{"baseURL": "https://mine.example.com/v1"},
			},
			"builtin:zai": map[string]interface{}{
				"name":    "Z.ai - API Key",
				"kind":    "openai-compatible",
				"options": map[string]interface{}{"baseURL": "https://zai.example.com/v1"},
			},
			"account:work": map[string]interface{}{
				"name":    "Work Account",
				"kind":    "anthropic",
				"options": map[string]interface{}{"baseURL": "https://work.example.com/v1"},
			},
		},
	}

	preview, err := PreviewLegacyImport(legacyDoc, nil)
	if err != nil {
		t.Fatalf("PreviewLegacyImport：%v", err)
	}
	if len(preview.Providers) != 1 || preview.Providers[0].ID != "mine" {
		t.Fatalf("待导入列表应只有 mine，得到 %+v", preview.Providers)
	}

	dir := t.TempDir()
	v2Path := filepath.Join(dir, "provider_config.json")
	if err := os.WriteFile(v2Path, canonicalZCodeV2(t), 0644); err != nil {
		t.Fatal(err)
	}
	v2Doc, err := LoadConfig(v2Path)
	if err != nil {
		t.Fatalf("读取新版文件：%v", err)
	}
	newDoc, report, err := ApplyLegacyImport(v2Doc, legacyDoc, []string{"mine", "builtin:zai", "account:work"}, v2Path)
	if err != nil {
		t.Fatalf("ApplyLegacyImport：%v", err)
	}
	if strings.Join(report.Imported, ",") != "mine" {
		t.Errorf("imported 应为 [mine]，得到 %v", report.Imported)
	}
	if strings.Join(report.Skipped, ",") != "builtin:zai,account:work" {
		t.Errorf("skipped 应为 [builtin:zai account:work]，得到 %v", report.Skipped)
	}
	cfg := zcodeprovider.NewConfigFromDoc(newDoc)
	if cfg.ProviderRule("builtin:zai") != nil || cfg.ProviderRule("account:work") != nil {
		t.Error("内建/账号型供应商不应被写成个人规则")
	}
}

func TestGetBackendZCodeFormatRouting(t *testing.T) {
	dir := t.TempDir()

	// 旧版：带 JSONC 注释的 config.json。
	legacyPath := filepath.Join(dir, "config.json")
	writeTestFile(t, legacyPath, "{\n// legacy comment\n\"provider\":{\"a\":{\"name\":\"A\"}}\n}\n")

	// 新版：canonical 内容 + 一个未知顶层键（仍是 v2，但严格解码必须失败）。
	v2BrokenPath := filepath.Join(dir, "broken_provider_config.json")
	clean := strings.TrimSuffix(string(canonicalZCodeV2(t, "a")), "\n")
	writeTestFile(t, v2BrokenPath, strings.TrimSuffix(clean, "}")+`,"zzz":1}`+"\n")

	v2Path := filepath.Join(dir, "provider_config.json")
	if err := os.WriteFile(v2Path, canonicalZCodeV2(t, "a"), 0644); err != nil {
		t.Fatal(err)
	}

	legacyBackend, err := GetBackend(string(AgentZCode), legacyPath)
	if err != nil {
		t.Fatalf("legacy 文件应拿到 backend：%v", err)
	}
	if _, err := legacyBackend.Read(legacyPath); err != nil {
		t.Fatalf("legacy backend 应能读取 JSONC：%v", err)
	}
	legacyOut := filepath.Join(dir, "out_legacy.json")
	if err := legacyBackend.Write(legacyOut, Doc{"provider": map[string]interface{}{"b": map[string]interface{}{"name": "B"}}}, ""); err != nil {
		t.Fatalf("legacy backend 应能写出旧版文档：%v", err)
	}

	v2Backend, err := GetBackend(string(AgentZCode), v2Path)
	if err != nil {
		t.Fatalf("v2 文件应拿到 backend：%v", err)
	}
	doc, err := v2Backend.Read(v2Path)
	if err != nil {
		t.Fatalf("v2 backend 读取失败：%v", err)
	}
	if len(zcodeprovider.NewConfigFromDoc(doc).ProviderRules()) != 1 {
		t.Error("v2 backend 应读出 1 条 providerRule")
	}
	outV2 := filepath.Join(dir, "out_v2.json")
	if err := v2Backend.Write(outV2, doc, ""); err != nil {
		t.Fatalf("v2 backend 写出失败：%v", err)
	}
	roundTripped, err := v2Backend.Read(outV2)
	if err != nil {
		t.Fatalf("v2 backend 回读失败：%v", err)
	}
	if zcodeprovider.NewConfigFromDoc(roundTripped).ProviderRule("a") == nil {
		t.Error("v2 backend 写出的文件应能回读出 providerRule a")
	}
	if err := v2Backend.Write(filepath.Join(dir, "out_v2_bad.json"), Doc{"provider": map[string]interface{}{}}, ""); err == nil {
		t.Error("v2 backend 应拒绝非 canonical 文档")
	}

	// 同一个目标文件只按内容分流：未知键的 v2 文件走 v2 backend -> 严格解码报错。
	brokenBackend, err := GetBackend(string(AgentZCode), v2BrokenPath)
	if err != nil {
		t.Fatalf("未知键的 v2 文件也应拿到 backend：%v", err)
	}
	if _, err := brokenBackend.Read(v2BrokenPath); err == nil {
		t.Error("未知键的 v2 文件应被 v2 backend 拒绝")
	}

	// 缺失文件 / 空文件判定为 unknown，回落 legacy backend。
	missingBackend, err := GetBackend(string(AgentZCode), filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatalf("缺失文件应回落 legacy backend：%v", err)
	}
	if err := missingBackend.Write(filepath.Join(dir, "out_missing.json"), Doc{"provider": map[string]interface{}{}}, ""); err != nil {
		t.Errorf("缺失文件走的应是 legacy backend（应能写旧版文档）：%v", err)
	}

	// 其它 agent 的既有行为不变：未注册 backend -> 报错。
	for _, agentID := range []string{"opencode", "deepseek", "opvcode", ""} {
		if _, err := GetBackend(agentID, v2Path); err == nil {
			t.Errorf("agent「%s」不应有 backend", agentID)
		}
	}
}

func TestZCodeV2StoreRoundTrip(t *testing.T) {
	doc := zcodeV2DocForStore(t)
	store := zcodeV2Backend{}.Store(doc)

	summaries := store.List(doc)
	if len(summaries) != 1 {
		t.Fatalf("List 应返回 1 条摘要，得到 %+v", summaries)
	}
	if s := summaries[0]; s.ID != "x" || s.Kind != "openai-compatible" || !s.HasAPIKey || s.ModelCount != 1 {
		t.Errorf("摘要不正确：%+v", s)
	}

	edit, err := store.Get(doc, "x")
	if err != nil {
		t.Fatalf("Get：%v", err)
	}
	if edit.Name != "X" || edit.APIKey != "sk-x" || edit.Kind != "openai-compatible" || !edit.APIKeyRequired {
		t.Errorf("ProviderEdit 不正确：%+v", edit)
	}
	if len(edit.Cards) != 1 || edit.Cards[0].ModelID != "m1" || edit.Cards[0].Context != 200000 {
		t.Fatalf("卡片不正确：%+v", edit.Cards)
	}
	if !edit.Cards[0].Reasoning || strings.Join(edit.Cards[0].Variants, ",") != "off,high" {
		t.Errorf("推理档位不正确：%+v", edit.Cards[0])
	}

	if _, err := store.Get(doc, "nope"); err == nil {
		t.Error("不存在的提供商应返回错误")
	}

	// 保存：本包未建模的合法字段必须原样保留。
	updated, err := store.Upsert(doc, "x", ProviderPayload{
		"id":       edit.ID,
		"name":     "X Renamed",
		"kind":     edit.Kind,
		"base_url": "https://x2.example.com/v1",
		"api_key":  edit.APIKey,
		"cards":    edit.Cards,
	}, SaveOptions{})
	if err != nil {
		t.Fatalf("Upsert：%v", err)
	}
	cfg := zcodeprovider.NewConfigFromDoc(updated)
	rule := cfg.ProviderRule("x")
	if rule == nil {
		t.Fatal("Upsert 后应仍有 providerRule x")
	}
	if rule["templateId"] != "tpl-1" || rule["enabled"] != true {
		t.Errorf("providerRule 级未建模字段应保留：%+v", rule)
	}
	if name := zcodeprovider.RuleProviderName(rule); name != "X Renamed" {
		t.Errorf("providerName 应更新为 X Renamed，得到 %q", name)
	}
	providerCfg := zcodeprovider.RuleConfig(rule)
	if providerCfg["logo"] == nil || providerCfg["visibility"] != "visible" || providerCfg["modelOrder"] == nil {
		t.Errorf("config 级未建模字段应保留：%+v", providerCfg)
	}
	if baseURL := zcodeprovider.ProviderAPIBaseURL(providerCfg); baseURL != "https://x2.example.com/v1" {
		t.Errorf("baseUrl 应更新，得到 %q", baseURL)
	}
	if headers := zcodeprovider.ProviderAPIHeaders(providerCfg); headers["X-Test"] != "1" {
		t.Errorf("api.headers 应保留：%+v", headers)
	}
	if url := zcodeprovider.ProviderAccessAPIKeyManagementURL(providerCfg); url != "https://x.example.com/keys" {
		t.Errorf("apiKeyManagementUrl 应保留，得到 %q", url)
	}
	if ids := strings.Join(zcodeprovider.ProviderPersonalModelIDs(providerCfg), ","); ids != "m1" {
		t.Errorf("personalModelIds 应为 m1，得到 %q", ids)
	}
	if order := strings.Join(cfg.ProviderOrder(), ","); order != "x" {
		t.Errorf("providerOrder 不应重复追加：%q", order)
	}
	mcfg := zcodeprovider.ModelRuleConfig(cfg.ProviderModelRule("x", "m1"))
	if enabled, ok := zcodeprovider.ModelEnabled(mcfg); !ok || !enabled {
		t.Errorf("模型规则 config.enabled 应保留：%+v", mcfg)
	}
	reasoning, _ := zcodeprovider.ModelOptionSpecs(mcfg)["reasoningLevel"].(map[string]interface{})
	if reasoning["map"] != "custom-map" {
		t.Errorf("reasoningLevel.map 应保留：%+v", reasoning)
	}
	if _, err := zcodeprovider.Encode(cfg); err != nil {
		t.Errorf("Upsert 结果应能 canonical 编码：%v", err)
	}

	// 重命名：整条规则搬走，providerOrder 同步；占用已有 ID 时拒绝。
	renamed, err := store.Upsert(updated, "x", ProviderPayload{
		"id":       "y",
		"name":     edit.Name,
		"kind":     edit.Kind,
		"base_url": edit.BaseURL,
		"api_key":  edit.APIKey,
		"cards":    edit.Cards,
	}, SaveOptions{})
	if err != nil {
		t.Fatalf("Upsert 重命名：%v", err)
	}
	renamedDoc := zcodeprovider.NewConfigFromDoc(renamed)
	if renamedDoc.ProviderRule("x") != nil || renamedDoc.ProviderRule("y") == nil {
		t.Error("重命名应把 x 的规则迁到 y")
	}
	if order := strings.Join(renamedDoc.ProviderOrder(), ","); order != "y" {
		t.Errorf("providerOrder 应同步为 y，得到 %q", order)
	}
	if renamedDoc.ProviderModelRule("y", "m1") == nil {
		t.Error("模型规则应随 providerId 一起改名")
	}
	if renamedDoc.ProviderRule("y")["templateId"] != "tpl-1" {
		t.Error("重命名应保留本包未建模的字段")
	}
	if _, err := store.Upsert(renamed, "y", ProviderPayload{
		"id":       "x",
		"name":     edit.Name,
		"kind":     edit.Kind,
		"base_url": edit.BaseURL,
		"api_key":  edit.APIKey,
		"cards":    edit.Cards,
	}, SaveOptions{}); err != nil {
		t.Fatalf("重命名到空闲 ID 应成功：%v", err)
	}
	if _, err := store.Upsert(renamed, "y", ProviderPayload{
		"id":       "x",
		"name":     edit.Name,
		"kind":     edit.Kind,
		"base_url": edit.BaseURL,
		"api_key":  edit.APIKey,
		"cards":    edit.Cards,
	}, SaveOptions{}); err == nil {
		t.Error("重命名到已存在的 ID 应报错")
	}

	// 删除模型：providerModelRule 与 personalModelIds 一并去掉。
	afterDelete, found, err := store.DeleteModel(updated, "x", "m1")
	if err != nil || !found {
		t.Fatalf("DeleteModel：found=%v err=%v", found, err)
	}
	after := zcodeprovider.NewConfigFromDoc(afterDelete)
	if after.ProviderModelRule("x", "m1") != nil {
		t.Error("模型规则应被删除")
	}
	if ids := zcodeprovider.ProviderPersonalModelIDs(zcodeprovider.RuleConfig(after.ProviderRule("x"))); len(ids) != 0 {
		t.Errorf("personalModelIds 应为空，得到 %v", ids)
	}
	if _, found, err := store.DeleteModel(updated, "x", "m2"); found || err != nil {
		t.Errorf("不存在的模型应返回 (false,nil)：found=%v err=%v", found, err)
	}
	if _, _, err := store.DeleteModel(updated, "nope", "m1"); err == nil {
		t.Error("provider 不存在时 DeleteModel 应返回错误")
	}

	// 删除 provider：规则、模型规则与 providerOrder 条目一起清理。
	afterProvider, found, err := store.Delete(updated, "x")
	if err != nil || !found {
		t.Fatalf("Delete：found=%v err=%v", found, err)
	}
	afterDel := zcodeprovider.NewConfigFromDoc(afterProvider)
	if afterDel.ProviderRule("x") != nil {
		t.Error("providerRule 应被删除")
	}
	if len(afterDel.ProviderModelRules()) != 0 {
		t.Error("该 provider 的模型规则应一并删除")
	}
	if len(afterDel.ProviderOrder()) != 0 {
		t.Errorf("providerOrder 条目应一并删除，得到 %v", afterDel.ProviderOrder())
	}
	if _, found, err := store.Delete(updated, "nope"); found || err != nil {
		t.Errorf("不存在的 provider 应返回 (false,nil)：found=%v err=%v", found, err)
	}
}

// canonicalV2DocForTest 返回只含给定 providerRule 的新版文档。
func canonicalV2DocForTest(t *testing.T, providerIDs ...string) Doc {
	t.Helper()
	cfg := zcodeprovider.NewConfig()
	for _, id := range providerIDs {
		cfg.UpsertProviderRule(zcodeprovider.NewProviderRule(id, id, zcodeprovider.GroupStandardPersonal))
	}
	cfg.SetProviderOrder(providerIDs)
	data, err := zcodeprovider.Encode(cfg)
	if err != nil {
		t.Fatalf("构造 v2 配置失败：%v", err)
	}
	decoded, err := zcodeprovider.Decode(data)
	if err != nil {
		t.Fatalf("解码自造 v2 配置失败：%v", err)
	}
	return decoded.Doc()
}

// zcodeV2DocForStore 构造一份带有多种“本包未建模但合法”字段的新版文档。
func zcodeV2DocForStore(t *testing.T) Doc {
	t.Helper()
	cfg := zcodeprovider.NewConfig()
	rule := zcodeprovider.NewProviderRule("x", "X", zcodeprovider.GroupStandardPersonal)
	rule["templateId"] = "tpl-1"
	rule["enabled"] = true
	providerCfg := zcodeprovider.RuleConfig(rule)
	providerCfg["logo"] = map[string]interface{}{"type": zcodeprovider.LogoTypeBuiltin, "key": "acme"}
	providerCfg["visibility"] = "visible"
	providerCfg["modelOrder"] = []interface{}{"m1"}
	providerCfg["api"] = map[string]interface{}{
		"type":    zcodeprovider.APITypeOpenAIChatCompletions,
		"baseUrl": "https://x.example.com/v1",
		"headers": map[string]interface{}{"X-Test": "1"},
	}
	providerCfg["access"] = map[string]interface{}{
		"type":                zcodeprovider.AccessTypeAPIKey,
		"apiKey":              "sk-x",
		"apiKeyManagementUrl": "https://x.example.com/keys",
	}
	providerCfg["personalModelIds"] = []interface{}{"m1"}
	cfg.UpsertProviderRule(rule)

	modelRule := zcodeprovider.NewProviderModelRule("x", "m1")
	modelCfg := zcodeprovider.RuleConfig(modelRule)
	modelCfg["enabled"] = true
	modelCfg["properties"] = map[string]interface{}{
		"contextWindow": 200000,
		"inputFormat":   map[string]interface{}{"supportsImage": true},
	}
	modelCfg["optionSpecs"] = map[string]interface{}{
		"reasoningLevel": map[string]interface{}{
			"values": []interface{}{"disabled", "high"},
			"map":    "custom-map",
		},
	}
	cfg.UpsertProviderModelRule(modelRule)
	cfg.SetProviderOrder([]string{"x"})
	return cfg.Doc()
}

// TestZCodeAgentDefaultPathIsLegacy 锁住 AgentDefaultPath(AgentZCode) 的旧版语义。
// migrate.go 的 buildSide / MigrateExecuteAtPaths 与 DetectConfigLocationsForAgent 都用
// 它取路径后读 cfg["provider"]，而新版 provider_config.json 没有该键：一旦默认路径跟着
// 格式判定漂到新版，ZCode → 其他 Agent 的迁移页对 v2 用户就会显示 0 个供应商。
func TestZCodeAgentDefaultPathIsLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	// 新版路径由该环境变量决定，清空以免读到开发机上的真实配置。
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")

	legacyPath := ZCodeConfig()
	if legacyPath != filepath.Join(home, ".zcode", "v2", "config.json") {
		t.Fatalf("旧版路径应落在测试家目录下，得到 %q", legacyPath)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatal(err)
	}
	// 旧版：两个 provider，正是迁移页的数据源。
	writeTestFile(t, legacyPath, `{"provider":{"a":{"name":"A"},"b":{"name":"B"}}}`)
	// 同时放一份真实存在的新版文件：默认路径不能因此漂向新版。
	writeTestFile(t, ZCodeProviderConfigPath(), string(canonicalZCodeV2(t, "v2only")))

	if got := AgentDefaultPath(AgentZCode); got != legacyPath {
		t.Fatalf("AgentDefaultPath(AgentZCode) 应为旧版路径 %q，得到 %q", legacyPath, got)
	}
	cfg, err := LoadConfig(AgentDefaultPath(AgentZCode))
	if err != nil {
		t.Fatalf("按默认路径读取旧版配置失败：%v", err)
	}
	providers, ok := cfg["provider"].(map[string]interface{})
	if !ok {
		t.Fatalf("旧版配置应含 provider 对象，得到 %#v", cfg["provider"])
	}
	if len(providers) != 2 {
		t.Errorf("迁移页的数据源应读到 2 个 provider，得到 %d：%#v", len(providers), providers)
	}
}

// TestZCodeDefaultPathPrefersV2 锁住 app 层专用的“新版优先”语义。
func TestZCodeDefaultPathPrefersV2(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")

	v2Path := ZCodeProviderConfigPath()
	if err := os.MkdirAll(filepath.Dir(v2Path), 0755); err != nil {
		t.Fatal(err)
	}
	// 只有旧版文件：退回旧版。
	writeTestFile(t, ZCodeConfig(), `{"provider":{"a":{"name":"A"}}}`)
	if got := ZCodeDefaultPath(); got != ZCodeConfig() {
		t.Errorf("新版文件不存在时应返回旧版路径 %q，得到 %q", ZCodeConfig(), got)
	}
	// 新版文件存在且判定为 v2：优先新版。
	writeTestFile(t, v2Path, string(canonicalZCodeV2(t, "a")))
	if got := ZCodeDefaultPath(); got != v2Path {
		t.Errorf("新版文件存在时应返回新版路径 %q，得到 %q", v2Path, got)
	}
	// 两者都不存在：指向新版（新装用户即 v2）。
	if err := os.Remove(v2Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ZCodeConfig()); err != nil {
		t.Fatal(err)
	}
	if got := ZCodeDefaultPath(); got != v2Path {
		t.Errorf("两者都不存在时应返回新版路径 %q，得到 %q", v2Path, got)
	}
}

// TestDetectConfigLocationsIncludesZCodeV2 「选择配置」列表必须同时给出旧版与新版候选，
// 且新版条目要能显示出 provider 数量（v2 内容走 zcodeprovider 解码，而不是读 cfg["provider"]）。
func TestDetectConfigLocationsIncludesZCodeV2(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")

	legacyPath := ZCodeConfig()
	v2Path := ZCodeProviderConfigPath()
	if err := os.MkdirAll(filepath.Dir(v2Path), 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, legacyPath, `{"provider":{"legacy":{"name":"Legacy"}}}`)
	writeTestFile(t, v2Path, string(canonicalZCodeV2(t, "p1", "p2")))

	locs := DetectConfigLocationsForAgent(string(AgentZCode))
	var legacy, v2 *ConfigLocation
	for i := range locs {
		switch locs[i].Path {
		case legacyPath:
			legacy = &locs[i]
		case v2Path:
			v2 = &locs[i]
		}
	}
	if legacy == nil {
		t.Fatalf("探测结果缺少旧版候选 %q：%+v", legacyPath, locs)
	}
	if v2 == nil {
		t.Fatalf("探测结果缺少新版候选 %q：%+v", v2Path, locs)
	}
	if v2.Label != "ZCode 新版配置" {
		t.Errorf("新版候选标签应为「ZCode 新版配置」，得到 %q", v2.Label)
	}
	if !v2.Exists || !v2.Accessible {
		t.Errorf("新版候选应存在且可读：%+v", *v2)
	}
	if v2.ProviderCount != 2 {
		t.Errorf("新版候选应显示 2 个供应商，得到 %d（Error=%q）", v2.ProviderCount, v2.Error)
	}
	if len(v2.ProviderIDs) != 2 {
		t.Errorf("新版候选应给出 2 个 provider id，得到 %v", v2.ProviderIDs)
	}
	if legacy.ProviderCount != 1 {
		t.Errorf("旧版候选应显示 1 个供应商，得到 %d（Error=%q）", legacy.ProviderCount, legacy.Error)
	}
}

// assertNoNullJSONArrayField 断言 v 编码成 JSON 后 keys 里的字段存在且不是 null。
// 该字段是前端直接 .map / .filter 的数组，null 会让整个页面报错。
func assertNoNullJSONArrayField(t *testing.T, label string, v interface{}, keys ...string) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s：json.Marshal 失败：%v", label, err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("%s：json.Unmarshal 失败：%v", label, err)
	}
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok {
			t.Fatalf("%s：JSON 里缺少字段 %q：%s", label, key, data)
		}
		if raw == nil {
			t.Errorf("%s：字段 %q 序列化为 null（应为 []）：%s", label, key, data)
		}
	}
}

// TestProviderReadPathNeverEmitsNullArrays 锁住“读取路径不向 JSON 输出 null 数组”。
// Go 的 encoding/json 把 nil slice 编成 null，而 ProviderEdit.cards / ProviderSummary
// 列表都没有 omitempty：provider 一条模型都没有（零模型 provider）时后端若返回 nil，
// 前端拿到的就是 null，会在 cards.map 上抛 “Cannot read properties of null”。
// 新版与旧版两条 backend 都要覆盖，且只碰 nil 语义、不改变长度语义。
func TestProviderReadPathNeverEmitsNullArrays(t *testing.T) {
	// 自检：确认“nil 切片 -> JSON null / 空非 nil 切片 -> []”这个前提仍成立，
	// 否则下面的断言会形同虚设。
	var nilCards []ModelCard
	emptyCards := make([]ModelCard, 0)
	for _, tc := range []struct {
		cards []ModelCard
		want  string
	}{{nilCards, `{"cards":null}`}, {emptyCards, `{"cards":[]}`}} {
		data, err := json.Marshal(struct {
			Cards []ModelCard `json:"cards"`
		}{tc.cards})
		if err != nil {
			t.Fatalf("前提自检：json.Marshal 失败：%v", err)
		}
		if string(data) != tc.want {
			t.Fatalf("前提自检：期望 %s，得到 %s", tc.want, data)
		}
	}

	// v2 backend：providerRule 存在，但一条 providerModelRule 都没有。
	v2Doc := canonicalV2DocForTest(t, "nomodels")
	edit, err := zcodeV2Store{}.Get(v2Doc, "nomodels")
	if err != nil {
		t.Fatalf("v2 Get(nomodels)：%v", err)
	}
	if edit.Cards == nil {
		t.Error("v2 零模型 provider 的 Cards 不应是 nil")
	}
	if len(edit.Cards) != 0 {
		t.Fatalf("v2 零模型 provider 的 Cards 应为空，得到 %+v", edit.Cards)
	}
	assertNoNullJSONArrayField(t, "v2 Get(nomodels)", edit, "cards")

	// legacy backend：同一缺陷的孪生位置。配置文件只写在 t.TempDir() 里。
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "config.json")
	writeTestFile(t, legacyPath, `{"provider":{"nomodels":{"name":"No Models","kind":"openai-compatible",`+
		`"options":{"baseURL":"https://nomodels.example.com/v1"}}}}`)
	backend, err := GetBackend(string(AgentZCode), legacyPath)
	if err != nil {
		t.Fatalf("取 legacy backend：%v", err)
	}
	if _, isV2 := backend.(zcodeV2Backend); isV2 {
		t.Fatal("只含 provider 键的旧版文件不应被判成 v2")
	}
	legacyDoc, err := backend.Read(legacyPath)
	if err != nil {
		t.Fatalf("读取旧版文件：%v", err)
	}
	legacyEdit, err := backend.Store(legacyDoc).Get(legacyDoc, "nomodels")
	if err != nil {
		t.Fatalf("legacy Get(nomodels)：%v", err)
	}
	if legacyEdit.Cards == nil {
		t.Error("legacy 零模型 provider 的 Cards 不应是 nil")
	}
	if len(legacyEdit.Cards) != 0 {
		t.Fatalf("legacy 零模型 provider 的 Cards 应为空，得到 %+v", legacyEdit.Cards)
	}
	assertNoNullJSONArrayField(t, "legacy Get(nomodels)", legacyEdit, "cards")

	// legacy List：连 provider 键都没有的空配置也必须给 [] 而不是 null。
	for _, cfg := range []map[string]interface{}{
		nil,
		{},
		{"provider": map[string]interface{}{}},
	} {
		summaries := BuildProviderSummary(cfg)
		if summaries == nil {
			t.Errorf("BuildProviderSummary(%#v) 不应返回 nil", cfg)
			continue
		}
		data, err := json.Marshal(summaries)
		if err != nil {
			t.Fatalf("BuildProviderSummary(%#v)：json.Marshal 失败：%v", cfg, err)
		}
		if string(data) != "[]" {
			t.Errorf("BuildProviderSummary(%#v) 应序列化为 []，得到 %s", cfg, data)
		}
	}
}

// TestModelCardVariantsNeverEmitNull 锁住 CfgToCard 的 variants 字段。
// NormalizeVariants 对 nil / 全被过滤掉的输入返回 nil，所以 cards.go 里的初始化必须能
// 扛住这次重新赋值；同时取值语义不能变：有档位照样返回档位，长度判断保持原样。
func TestModelCardVariantsNeverEmitNull(t *testing.T) {
	cases := []struct {
		label    string
		modelCfg map[string]interface{}
	}{
		{"没有 reasoning 键", map[string]interface{}{"name": "m"}},
		{"reasoning 存在但没有 variants", map[string]interface{}{"reasoning": map[string]interface{}{"enabled": true}}},
		{"variants 全被过滤掉", map[string]interface{}{"reasoning": map[string]interface{}{"variants": []interface{}{"not-a-variant"}}}},
		{"reasoning 是布尔", map[string]interface{}{"reasoning": false}},
	}
	for _, tc := range cases {
		card := CfgToCard("m", tc.modelCfg)
		if card.Variants == nil {
			t.Errorf("%s：Variants 不应是 nil", tc.label)
		}
		if len(card.Variants) != 0 {
			t.Errorf("%s：Variants 应为空，得到 %v", tc.label, card.Variants)
		}
		assertNoNullJSONArrayField(t, tc.label, card, "variants")
	}

	// 非空输入：取值与长度语义与改动前一致。
	card := CfgToCard("m", map[string]interface{}{
		"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"off", "high"}},
	})
	if strings.Join(card.Variants, ",") != "off,high" {
		t.Errorf("variants 应保持 off,high，得到 %v", card.Variants)
	}
	if !card.Reasoning {
		t.Error("off+high 档位时 Reasoning 应为 true")
	}
	card = CfgToCard("m", map[string]interface{}{
		"reasoning": map[string]interface{}{"variants": []interface{}{"off"}},
	})
	if strings.Join(card.Variants, ",") != "off" {
		t.Errorf("variants 应为 off，得到 %v", card.Variants)
	}
	if card.Reasoning {
		t.Error("只有 off 档位时 Reasoning 应为 false")
	}
	// 同一张卡经编辑器回写后仍能编码出非空 variants 列表。
	enabled := CfgToCard("m", map[string]interface{}{
		"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"off", "high"}},
	})
	cfg, _, err := CardToCfg(enabled, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg：%v", err)
	}
	reasoning, _ := cfg["reasoning"].(map[string]interface{})
	if reasoning == nil {
		t.Fatalf("CardToCfg 应写出 reasoning：%+v", cfg)
	}
	if got, ok := reasoning["variants"].([]string); !ok || strings.Join(got, ",") != "off,high" {
		t.Errorf("CardToCfg 回写的 variants 不正确：%+v", reasoning["variants"])
	}
}
