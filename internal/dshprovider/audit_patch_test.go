package dshprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

// auditJSPatch 故意把注释与 !!js 放在 llm-pi-ai 的 config 里：
// 旧实现会解码整个 config 再编码，静默丢掉这些内容。
const auditJSPatch = `# patch header
- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    # 动态 baseURL，不能丢
    baseURL: !!js process.env.ACME_BASE
    apiKeyEnv: ACME_API_KEY
    providers:
      acme:
        # 第一条路由
        displayName: Acme
        baseURL: https://acme.example/v1
- id: agent-default-model
  config:
    # 默认模型
    provider: acme
    model: acme-1
- id: tools
  config:
    mode: !!js process.env.DSH_TOOLS_MODE
`

func TestSetProvidersKeepsConfigJSTagsCommentsAndOrder(t *testing.T) {
	p, err := LoadPatch([]byte(auditJSPatch))
	if err != nil {
		t.Fatalf("LoadPatch: %v", err)
	}
	providers, err := p.Providers()
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	providers["second"] = map[string]interface{}{"baseURL": "https://second.example"}
	if err := p.SetProviders(providers); err != nil {
		t.Fatalf("SetProviders: %v", err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	text := string(out)

	if !strings.Contains(text, "!!js process.env.ACME_BASE") {
		t.Fatalf("config 级的 !!js 表达式被写没了：\n%s", text)
	}
	if !strings.Contains(text, "# 动态 baseURL") {
		t.Fatalf("config 级注释被写没了：\n%s", text)
	}
	if !strings.Contains(text, "# 第一条路由") {
		t.Fatalf("未改动的 provider 条目里的注释被写没了：\n%s", text)
	}
	if !strings.Contains(text, "!!js process.env.DSH_TOOLS_MODE") {
		t.Fatalf("其它条目的 !!js 被写没了：\n%s", text)
	}
	// 键序：config 里 baseURL 仍在 apiKeyEnv 之前（旧实现会按字母序重排）。
	if i, j := strings.Index(text, "baseURL: !!js"), strings.Index(text, "apiKeyEnv: ACME_API_KEY"); i < 0 || j < 0 || i > j {
		t.Fatalf("config 键序被改动：\n%s", text)
	}

	reloaded, err := LoadPatch(out)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	back, err := reloaded.Providers()
	if err != nil {
		t.Fatalf("reload Providers: %v", err)
	}
	if len(back) != 2 {
		t.Fatalf("providers = %d, want 2: %v", len(back), back)
	}
	acme, _ := back["acme"].(map[string]interface{})
	if acme == nil || acme["baseURL"] != "https://acme.example/v1" {
		t.Fatalf("acme 被破坏：%v", back["acme"])
	}
	if _, ok := back["second"]; !ok {
		t.Fatalf("新增 provider 丢失：%v", back)
	}
}

func TestSetProvidersRemovesAndReplaces(t *testing.T) {
	p, err := LoadPatch([]byte(auditJSPatch))
	if err != nil {
		t.Fatalf("LoadPatch: %v", err)
	}
	if err := p.SetProviders(map[string]interface{}{
		"acme": map[string]interface{}{"baseURL": "https://changed.example"},
	}); err != nil {
		t.Fatalf("SetProviders: %v", err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	reloaded, err := LoadPatch(out)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	back, err := reloaded.Providers()
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	if len(back) != 1 {
		t.Fatalf("providers = %v, want only acme", back)
	}
	acme, _ := back["acme"].(map[string]interface{})
	if acme["baseURL"] != "https://changed.example" {
		t.Fatalf("acme 未被替换：%v", acme)
	}
	if strings.Contains(string(out), "displayName") {
		t.Fatalf("被替换的条目仍残留旧键：\n%s", out)
	}
}

func TestSetAgentDefaultModelKeepsComments(t *testing.T) {
	p, err := LoadPatch([]byte(auditJSPatch))
	if err != nil {
		t.Fatalf("LoadPatch: %v", err)
	}
	def := p.AgentDefaultModel()
	if def == nil {
		t.Fatal("AgentDefaultModel = nil")
	}
	def["model"] = "acme-2"
	if err := p.SetAgentDefaultModel(def); err != nil {
		t.Fatalf("SetAgentDefaultModel: %v", err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "# 默认模型") {
		t.Fatalf("agent-default-model 注释被写没了：\n%s", text)
	}
	if !strings.Contains(text, "model: acme-2") {
		t.Fatalf("agent-default-model 未更新：\n%s", text)
	}
	if !strings.Contains(text, "provider: acme") {
		t.Fatalf("未改动的键丢失：\n%s", text)
	}
}

// backend 重建 provider 时会走一次 JSON 往返，数字会从 int 变成 float64。
// 这种情况也必须判定为“值没变”，否则未改动的 provider 注释仍会被写没。
func TestSetProvidersKeepsCommentsWhenValuesRoundTrippedThroughJSON(t *testing.T) {
	doc := `- id: llm-pi-ai
  config:
    providers:
      acme:
        # 模型注释
        models:
          - id: a1
            contextWindow: 262144
`
	p, err := LoadPatch([]byte(doc))
	if err != nil {
		t.Fatalf("LoadPatch: %v", err)
	}
	providers, err := p.Providers()
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	raw, err := json.Marshal(providers["acme"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundTripped map[string]interface{}
	if err := json.Unmarshal(raw, &roundTripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	providers["acme"] = roundTripped
	if err := p.SetProviders(providers); err != nil {
		t.Fatalf("SetProviders: %v", err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.Contains(string(out), "# 模型注释") {
		t.Fatalf("JSON 往返后的未改动 provider 注释被写没了：\n%s", out)
	}
}
