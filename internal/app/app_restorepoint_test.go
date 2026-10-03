package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
	"agentprovidermanager/internal/zcodeprovider"
)

// App 层的“每次变更都留还原点”集成测试。
// 环境全部沙箱化（LOCALAPPDATA / USERPROFILE / ZCODE_PERSONAL_PROVIDER_CONFIG_FILE），
// 不会读写任何真实用户配置。

func sandboxApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("USERPROFILE", home)
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")
	return NewApp()
}

func writeText(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入 %s 失败：%v", path, err)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	return string(data)
}

func requireSuccess(t *testing.T, resp map[string]interface{}, what string) {
	t.Helper()
	if ok, _ := resp["success"].(bool); !ok {
		t.Fatalf("%s 失败：%v", what, resp["error"])
	}
}

func pointsFor(t *testing.T, target string) []core.RestorePoint {
	t.Helper()
	return core.ListRestorePoints(core.RestorePointFilter{TargetPath: target})
}

func snapshotContent(t *testing.T, point core.RestorePoint) string {
	t.Helper()
	path, err := core.RestorePointContentPath(point.ID)
	if err != nil {
		t.Fatalf("还原点内容路径失败：%v", err)
	}
	return readText(t, path)
}

const legacyProviderFixture = `{
  "provider": {
    "acme": {
      "name": "Acme",
      "kind": "openai-compatible",
      "options": {"baseURL": "https://acme.example.invalid/v1", "apiKey": "sk-test"},
      "models": {"m1": {"limit": {"context": 1000}}}
    }
  }
}`

// 保存 / 删除模型 / 删除供应商 三条路径都必须留下还原点，且保存的是改动前的内容。
func TestAppMutationsCreateRestorePoints(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeConfig()
	writeText(t, target, legacyProviderFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	resp := a.SaveProvider("acme", map[string]interface{}{
		"id":       "acme",
		"name":     "Acme Renamed",
		"kind":     "openai-compatible",
		"base_url": "https://acme.example.invalid/v1",
		"api_key":  "sk-test",
		"cards":    []core.ModelCard{{ModelID: "m1", Context: 2000}},
	})
	requireSuccess(t, resp, "SaveProvider")
	points := pointsFor(t, target)
	if len(points) != 1 {
		t.Fatalf("保存后应有 1 个还原点，实际 %d", len(points))
	}
	if points[0].Operation != core.OpSaveProvider || points[0].ProviderID != "acme" {
		t.Errorf("保存还原点元数据不正确：%+v", points[0])
	}
	if points[0].Format != core.FormatLegacy || !points[0].Existed {
		t.Errorf("格式/存在标记不正确：%+v", points[0])
	}
	if got := snapshotContent(t, points[0]); got != legacyProviderFixture {
		t.Errorf("还原点应保存改动前的原始字节，得到 %q", got)
	}
	if !strings.Contains(readText(t, target), "Acme Renamed") {
		t.Error("保存后目标文件应已更新")
	}

	requireSuccess(t, a.DeleteModel("acme", "m1"), "DeleteModel")
	points = pointsFor(t, target)
	if len(points) != 2 || points[0].Operation != core.OpDeleteModel {
		t.Fatalf("删除模型后应有 2 个还原点（最新的为删除模型），实际 %+v", points)
	}
	if points[0].ModelID != "m1" {
		t.Errorf("删除模型的还原点应记录 model_id，得到 %+v", points[0])
	}

	requireSuccess(t, a.DeleteProvider("acme"), "DeleteProvider")
	points = pointsFor(t, target)
	if len(points) != 3 || points[0].Operation != core.OpDeleteProvider {
		t.Fatalf("删除供应商后应有 3 个还原点，实际 %+v", points)
	}
	for _, point := range points {
		if point.SHA256 == "" || point.SizeBytes == 0 {
			t.Errorf("还原点应记录内容摘要与大小：%+v", point)
		}
	}

	// 回滚到最早那个还原点：内容应回到最初。
	outcome, err := core.RestoreRestorePoint(points[len(points)-1].ID)
	if err != nil {
		t.Fatalf("回滚失败：%v", err)
	}
	if outcome.Snapshot == nil {
		t.Error("回滚前应自动建立还原点")
	}
	if got := readText(t, target); got != legacyProviderFixture {
		t.Errorf("回滚后应回到最初内容，得到 %q", got)
	}
}

// 目标文件原本不存在时：还原点记录“当时不存在”，并且删除供应商这类操作不会先失败。
func TestAppSaveCreatesPointWhenFileAbsent(t *testing.T) {
	a := sandboxApp(t)
	target := filepath.Join(core.ZCodeDir(), "config.json")
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	resp := a.SaveProvider("acme", map[string]interface{}{
		"id":       "acme",
		"name":     "Acme",
		"kind":     "anthropic",
		"base_url": "https://acme.example.invalid",
		"api_key":  "sk-test",
		"cards":    []core.ModelCard{{ModelID: "m1"}},
	})
	requireSuccess(t, resp, "SaveProvider")
	points := pointsFor(t, target)
	if len(points) != 1 {
		t.Fatalf("应有 1 个还原点，实际 %d", len(points))
	}
	if points[0].Existed {
		t.Errorf("文件原本不存在，Existed 应为 false：%+v", points[0])
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("保存后目标文件应存在：%v", err)
	}
	// 回滚该还原点 = 回到“没有这个文件”的状态。
	outcome, err := core.RestoreRestorePoint(points[0].ID)
	if err != nil {
		t.Fatalf("回滚失败：%v", err)
	}
	if outcome.RemovedPath == "" {
		t.Error("回滚到不存在状态时应移走当前文件")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("回滚后目标文件应不存在，err = %v", err)
	}
}

// v2 兼容性检查与一键修复：修复会建立还原点，且回滚能回到修复前。
func TestAppRepairZCodeCompatCreatesRestorePoint(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	// 个人文件里出现 builtinModelIds：ZCode 3.14 会拒绝，工具应能读、能报、能修。
	broken := `{
  "schemaVersion": 1,
  "config": {
    "providerConfigRules": {"providerRules": [
      {"providerId": "acme", "config": {"group": "zai-family", "builtinModelIds": ["acme-builtin"], "personalModelIds": ["m1"]}}
    ]},
    "modelConfigRules": {"providerModelRules": [], "manualProviderModelRules": []}
  }
}`
	writeText(t, target, broken)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	if !core.IsZCodeV2Path(target) {
		t.Fatal("前提：目标文件应被识别为 v2")
	}
	compat := a.GetZCodeCompat()
	requireSuccess(t, compat, "GetZCodeCompat")
	issues, _ := compat["issues"].([]zcodeprovider.CompatibilityIssue)
	if len(issues) < 2 {
		t.Fatalf("应报告 builtinModelIds 与 group 两处问题，实际 %v", compat["issues"])
	}

	repaired := a.RepairZCodeCompat()
	requireSuccess(t, repaired, "RepairZCodeCompat")
	fixes, _ := repaired["fixes"].([]string)
	if len(fixes) == 0 {
		t.Fatalf("应返回修复动作清单：%v", repaired)
	}
	if remaining, _ := repaired["issues"].([]zcodeprovider.CompatibilityIssue); len(remaining) != 0 {
		t.Errorf("修复后不应残留问题：%v", remaining)
	}
	after := readText(t, target)
	if strings.Contains(after, "builtinModelIds") {
		t.Error("builtinModelIds 应已被移除")
	}
	if !strings.Contains(after, "acme-builtin") {
		t.Error("builtinModelIds 的值应合并进 personalModelIds 而不是丢弃")
	}
	if !strings.Contains(after, `"standard-personal"`) {
		t.Error("家族 group 应被改成 standard-personal")
	}

	points := pointsFor(t, target)
	if len(points) != 1 {
		t.Fatalf("修复应留下 1 个还原点，实际 %d", len(points))
	}
	if points[0].Operation != core.OpManual || !strings.Contains(points[0].Note, "兼容性") {
		t.Errorf("修复还原点元数据不正确：%+v", points[0])
	}
	if got := snapshotContent(t, points[0]); got != broken {
		t.Error("还原点应保存修复前的原始内容")
	}
	if _, err := core.RestoreRestorePoint(points[0].ID); err != nil {
		t.Fatalf("回滚修复失败：%v", err)
	}
	if got := readText(t, target); got != broken {
		t.Error("回滚后应回到修复前的内容")
	}
}

// 兼容性检查对非 v2 / 不存在的目标必须安静返回，不误报。
func TestGetZCodeCompatQuietOnOtherTargets(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeConfig()
	writeText(t, target, legacyProviderFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	compat := a.GetZCodeCompat()
	requireSuccess(t, compat, "GetZCodeCompat")
	if issues, _ := compat["issues"].([]zcodeprovider.CompatibilityIssue); len(issues) != 0 {
		t.Errorf("legacy 目标不应报告 v2 兼容性问题：%v", issues)
	}
	if resp := a.RepairZCodeCompat(); resp["success"] == true {
		t.Errorf("非 v2 目标不应执行修复：%v", resp)
	}
}
