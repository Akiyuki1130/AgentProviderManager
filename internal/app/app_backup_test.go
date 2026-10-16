package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentprovidermanager/internal/core"
	appupdate "agentprovidermanager/internal/update"
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

func writeBackupFile(t *testing.T, path string, providers []core.ProviderEdit) {
	t.Helper()
	raw, err := core.SealBackup(core.BackupPackage{
		Agent: string(core.AgentZCode), IncludeKeys: false, KeyMode: "plain",
		ProviderCount: len(providers), Providers: providers,
	}, "")
	if err != nil {
		t.Fatalf("构造备份文件失败：%v", err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatalf("写入备份文件失败：%v", err)
	}
}

// 「不含密钥」的备份把本地真实 Key 复用到文件里的 baseUrl 上等于把凭据交给对方：
// 来源不同必须整条跳过，来源相同才沿用本地 Key（并保留原有便利）。
func TestImportBackupDoesNotReuseKeyAcrossOrigins(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	writeText(t, target, backupV2Fixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)
	dir := t.TempDir()

	foreign := filepath.Join(dir, "foreign.json")
	writeBackupFile(t, foreign, []core.ProviderEdit{{
		ID: "acme", Name: "Acme", Kind: "openai-compatible",
		BaseURL: "https://evil.example.invalid/v1", Cards: []core.ModelCard{{ModelID: "m1"}},
	}})
	inspect := a.InspectBackup(foreign, "")
	requireSuccess(t, inspect, "InspectBackup")
	preview, _ := inspect["providers"].([]map[string]interface{})
	if len(preview) != 1 || preview[0]["base_url"] != "https://evil.example.invalid/v1" {
		t.Fatalf("预览必须显示 base_url：%#v", inspect["providers"])
	}
	resp := a.ImportBackup(foreign, "", "overwrite")
	requireSuccess(t, resp, "ImportBackup(foreign)")
	if imported, _ := resp["imported"].([]string); len(imported) != 0 {
		t.Errorf("来源不同的无密钥覆盖应被跳过，实际导入 %v", imported)
	}
	conflicts, _ := resp["key_conflict"].([]map[string]interface{})
	if len(conflicts) != 1 || conflicts[0]["id"] != "acme" {
		t.Fatalf("应报告 key_conflict：%v", resp["key_conflict"])
	}
	provider := a.GetProvider("acme")
	edit, _ := provider["provider"].(*core.ProviderEdit)
	if edit == nil || edit.APIKey != "sk-placeholder" || !strings.Contains(edit.BaseURL, "acme.example.invalid") {
		t.Fatalf("本地提供商不应被改动：%#v", provider["provider"])
	}

	// 同源（仅路径不同）的无密钥备份：沿用本地 Key，正常导入。
	sameOrigin := filepath.Join(dir, "same-origin.json")
	writeBackupFile(t, sameOrigin, []core.ProviderEdit{{
		ID: "acme", Name: "Acme", Kind: "openai-compatible",
		BaseURL: "https://acme.example.invalid/openai/v1", Cards: []core.ModelCard{{ModelID: "m1"}},
	}})
	resp = a.ImportBackup(sameOrigin, "", "overwrite")
	requireSuccess(t, resp, "ImportBackup(same origin)")
	if imported, _ := resp["imported"].([]string); len(imported) != 1 {
		t.Fatalf("同源应正常导入：%v", resp)
	}
	provider = a.GetProvider("acme")
	edit, _ = provider["provider"].(*core.ProviderEdit)
	if edit == nil || edit.APIKey != "sk-placeholder" {
		t.Fatalf("同源导入应沿用本地 Key：%#v", provider["provider"])
	}
}

// 备份可能含凭据，读写都不接受 UNC 与设备路径（那是把凭据投递到第三方主机）。
func TestBackupRejectsUNCAndDevicePaths(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	writeText(t, target, backupV2Fixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	for _, path := range []string{
		`\\server\share\backup.json`,
		`//server/share/backup.json`,
		`\\?\C:\temp\backup.json`,
	} {
		if resp := a.ExportBackup(map[string]interface{}{"path": path, "include_keys": false, "key_mode": "plain"}); resp["success"] == true {
			t.Errorf("导出不应接受 %q", path)
		}
		if resp := a.InspectBackup(path, ""); resp["success"] == true {
			t.Errorf("查看不应接受 %q", path)
		}
		if resp := a.ImportBackup(path, "", "overwrite"); resp["success"] == true {
			t.Errorf("导入不应接受 %q", path)
		}
	}
}

// 安装更新需要界面上显式确认换来的「一次性令牌」：有效令牌只放行一次，过期、错误或
// 未发放的令牌一律拒绝。这里直接驱动 mintInstallToken / consumeInstallToken，避免真的
// 启动 helper 进程或触碰真实路径；暂存路径用 t.TempDir() 下的占位值即可（令牌校验只
// 比对令牌本身，不读文件）。
func TestInstallUpdateRequiresSingleUseToken(t *testing.T) {
	a := sandboxApp(t)
	if resp := a.InstallUpdate("anything"); resp["success"] == true {
		t.Fatal("没有已下载的更新时不应接受安装")
	}
	if resp := a.PrepareUpdateInstall(); resp["success"] == true {
		t.Fatal("没有已下载的更新时不应发放令牌")
	}

	a.updateInfo = appupdate.UpdateInfo{
		StagedPath:     filepath.Join(t.TempDir(), ".apm-update-1.exe"),
		ExpectedSHA256: strings.Repeat("a", 64),
	}

	// 错误令牌必须被拒，并给出中文原因。
	if ok, reason := a.consumeInstallToken("bogus"); ok || reason == "" {
		t.Fatalf("错误令牌不应通过：ok=%v reason=%q", ok, reason)
	}
	// 未发放令牌时，任意串也不通过。
	if ok, _ := a.consumeInstallToken(""); ok {
		t.Fatal("未发放令牌时不应通过")
	}

	// 有效令牌放行一次。
	token, reason := a.mintInstallToken()
	if token == "" || reason != "" {
		t.Fatalf("应发放令牌：token=%q reason=%q", token, reason)
	}
	if ok, reason := a.consumeInstallToken(token); !ok || reason != "" {
		t.Fatalf("有效令牌应通过：ok=%v reason=%q", ok, reason)
	}
	// 第二次用同一枚必须失败：令牌是一次性的，消费后已作废。
	if ok, _ := a.consumeInstallToken(token); ok {
		t.Fatal("令牌是一次性的：第二次使用必须失败")
	}

	// 过期令牌：重新发放后把有效期回拨，必须失败且给出原因。
	token, reason = a.mintInstallToken()
	if token == "" || reason != "" {
		t.Fatalf("应再次发放令牌：token=%q reason=%q", token, reason)
	}
	a.installTokenExpiry = time.Now().Add(-time.Minute)
	if ok, reason := a.consumeInstallToken(token); ok || reason == "" {
		t.Fatalf("过期令牌不应通过：ok=%v reason=%q", ok, reason)
	}
}
