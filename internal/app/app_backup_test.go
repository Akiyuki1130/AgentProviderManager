package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
)

// apm-backup 导出 / 查看 / 导入的集成测试：环境全部沙箱化，只用占位 Key。

const backupV2Fixture = `{
  "schemaVersion": 1,
  "config": {
    "providerOrder": ["acme", "beta"],
    "providerConfigRules": {"providerRules": [
      {"providerId": "acme", "providerName": "Acme", "config": {
        "group": "standard-personal",
        "access": {"type": "api-key", "apiKey": "sk-placeholder"},
        "api": {"type": "openai-chat-completions", "baseUrl": "https://acme.example.invalid/v1"},
        "personalModelIds": ["m1"], "modelOrder": ["m1"]
      }},
      {"providerId": "beta", "providerName": "Beta", "config": {
        "group": "standard-personal",
        "access": {"type": "api-key", "apiKey": "sk-placeholder-2"},
        "api": {"type": "anthropic-messages", "baseUrl": "https://beta.example.invalid"},
        "personalModelIds": ["b1"], "modelOrder": ["b1"]
      }}
    ]},
    "modelConfigRules": {
      "providerModelRules": [
        {"providerId": "acme", "modelId": "m1", "config": {"properties": {"contextWindow": 1000}}},
        {"providerId": "beta", "modelId": "b1", "config": {"properties": {"contextWindow": 2000}}}
      ],
      "manualProviderModelRules": []
    }
  }
}`

func TestExportImportBackupRoundTrip(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	writeText(t, target, backupV2Fixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)
	dir := t.TempDir()

	// 不导 Key：信封明文，但不含任何凭据。
	plainPath := filepath.Join(dir, "plain.json")
	resp := a.ExportBackup(map[string]interface{}{"path": plainPath, "include_keys": false, "key_mode": "plain"})
	requireSuccess(t, resp, "ExportBackup(plain)")
	if n, _ := resp["provider_count"].(int); n != 2 {
		t.Fatalf("应导出 2 个 provider，实际 %#v", resp["provider_count"])
	}
	plain := readText(t, plainPath)
	if strings.Contains(plain, "sk-placeholder") {
		t.Error("不导 Key 的备份不应包含明文 Key")
	}
	if !strings.Contains(plain, "acme") {
		t.Error("不导 Key 的备份应保留 provider 本体")
	}

	// 密码模式：文件里不应出现明文 Key，空密码查看会被要求输入密码。
	pwPath := filepath.Join(dir, "pw.json")
	resp = a.ExportBackup(map[string]interface{}{"path": pwPath, "include_keys": true, "key_mode": "password", "password": "pw123"})
	requireSuccess(t, resp, "ExportBackup(password)")
	if strings.Contains(readText(t, pwPath), "sk-placeholder") {
		t.Error("密码备份不应包含明文 Key")
	}
	inspect := a.InspectBackup(pwPath, "")
	if ok, _ := inspect["success"].(bool); ok {
		t.Fatal("空密码查看应失败")
	}
	if needs, _ := inspect["needs_password"].(bool); !needs {
		t.Errorf("应返回 needs_password：%v", inspect)
	}
	inspect = a.InspectBackup(pwPath, "pw123")
	requireSuccess(t, inspect, "InspectBackup")
	providers, _ := inspect["providers"].([]map[string]interface{})
	if len(providers) != 2 || providers[0]["has_api_key"] != true {
		t.Fatalf("查看结果不正确：%v", inspect["providers"])
	}
	if inspect["agent_label"] != "ZCode" {
		t.Errorf("agent_label 应为 ZCode：%v", inspect["agent_label"])
	}

	// 清空当前配置后按覆盖导入：provider 与 Key 都回来，且导入自动留下还原点。
	if err := os.Remove(target); err != nil {
		t.Fatalf("删除测试配置失败：%v", err)
	}
	if before := len(pointsFor(t, target)); before != 0 {
		t.Fatalf("导入前不应有还原点：%d", before)
	}
	resp = a.ImportBackup(pwPath, "pw123", "overwrite")
	requireSuccess(t, resp, "ImportBackup")
	imported, _ := resp["imported"].([]string)
	if len(imported) != 2 {
		t.Fatalf("应导入 2 个 provider：%v", resp)
	}
	provider := a.GetProvider("acme")
	requireSuccess(t, provider, "GetProvider")
	edit, _ := provider["provider"].(*core.ProviderEdit)
	if edit == nil || edit.APIKey != "sk-placeholder" || len(edit.Cards) != 1 {
		t.Fatalf("导入后 provider 不正确：%v", provider["provider"])
	}
	if after := len(pointsFor(t, target)); after < 2 {
		t.Errorf("导入应自动建立还原点，实际 %d 个", after)
	}
}
