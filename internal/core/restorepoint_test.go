package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 测试一律使用临时目录与占位值，绝不触碰真实用户配置。

func restorePointHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("USERPROFILE", home)
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")
	return home
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	return string(data)
}

func createPoint(t *testing.T, ctx ChangeContext) *RestorePoint {
	t.Helper()
	point, err := CreateRestorePoint(ctx)
	if err != nil {
		t.Fatalf("CreateRestorePoint 失败：%v", err)
	}
	return point
}

// PrepareChange 必须同时产出同级 .bak_ 备份与还原点，且还原点保存的是改动前的原始字节。
func TestPrepareChangeSnapshotsOriginalBytes(t *testing.T) {
	home := restorePointHome(t)
	target := filepath.Join(home, ".zcode", "v2", "config.json")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	original := "{\n  // 注释与排版都必须原样保留\n  \"provider\": {}\n}\n"
	if err := os.WriteFile(target, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	snap, err := PrepareChange(ChangeContext{
		Target:     target,
		AgentID:    string(AgentZCode),
		Operation:  OpSaveProvider,
		ProviderID: "acme",
	})
	if err != nil {
		t.Fatalf("PrepareChange 失败：%v", err)
	}
	if snap.BackupPath == "" {
		t.Fatal("应产出同级 .bak_ 备份路径")
	}
	if _, err := os.Stat(snap.BackupPath); err != nil {
		t.Fatalf(".bak_ 备份不存在：%v", err)
	}
	point := snap.RestorePoint
	if point == nil {
		t.Fatal("应产出还原点")
	}
	if point.Operation != OpSaveProvider || point.ProviderID != "acme" {
		t.Errorf("元数据不正确：%+v", point)
	}
	if point.Format != FormatLegacy {
		t.Errorf("format = %q, want %q", point.Format, FormatLegacy)
	}
	if point.AgentID != string(AgentZCode) || point.AgentLabel == "" {
		t.Errorf("agent 元数据不正确：%+v", point)
	}
	if !point.Existed || point.SizeBytes != int64(len(original)) || point.SHA256 == "" || point.Fingerprint == "" {
		t.Errorf("文件应被完整记录：%+v", point)
	}
	contentPath, err := RestorePointContentPath(point.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustReadFile(t, contentPath); got != original {
		t.Errorf("还原点内容必须逐字节等于原文件，得到 %q", got)
	}
}

// 目标文件不存在时也要留一个“当时不存在”的还原点。
func TestCreateRestorePointWhenTargetMissing(t *testing.T) {
	home := restorePointHome(t)
	target := filepath.Join(home, ".zcode", "v2", "provider_config.json")
	point := createPoint(t, ChangeContext{Target: target, AgentID: string(AgentZCode), Operation: OpSaveProvider})
	if point.Existed {
		t.Error("文件不存在时 Existed 应为 false")
	}
	if point.Format != FormatV2 {
		t.Errorf("provider_config.json 应推断为 v2，得到 %q", point.Format)
	}
	if _, err := os.Stat(filepath.Join(RestorePointsDir(), point.ID, RestorePointContentName)); !os.IsNotExist(err) {
		t.Error("不存在状态不应写入 content")
	}
}

// 列表按时间倒序、可按目标路径与 Agent 过滤，损坏的 meta 不影响其它条目。
func TestListRestorePointsAndFilter(t *testing.T) {
	home := restorePointHome(t)
	a := filepath.Join(home, ".zcode", "v2", "config.json")
	b := filepath.Join(home, ".config", "opencode", "opencode.json")
	for _, path := range []string{a, b} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"provider":{}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	older := createPoint(t, ChangeContext{Target: a, AgentID: string(AgentZCode), Operation: OpSaveProvider})
	newer := createPoint(t, ChangeContext{Target: a, AgentID: string(AgentZCode), Operation: OpDeleteProvider})
	createPoint(t, ChangeContext{Target: b, AgentID: string(AgentOpenCode), Operation: OpSaveProvider})

	all := ListRestorePoints(RestorePointFilter{})
	if len(all) != 3 {
		t.Fatalf("全部还原点 = %d, want 3", len(all))
	}
	if all[0].ID <= all[1].ID {
		t.Errorf("列表应按 ID（时间）倒序：%v", []string{all[0].ID, all[1].ID})
	}
	byTarget := ListRestorePoints(RestorePointFilter{TargetPath: strings.ToUpper(a)})
	if len(byTarget) != 2 {
		t.Fatalf("按目标过滤 = %d, want 2（路径比较应忽略大小写）", len(byTarget))
	}
	byAgent := ListRestorePoints(RestorePointFilter{AgentID: string(AgentOpenCode)})
	if len(byAgent) != 1 {
		t.Fatalf("按 Agent 过滤 = %d, want 1", len(byAgent))
	}
	if newer.Operation != OpDeleteProvider || older.Operation != OpSaveProvider {
		t.Error("operation 未被记录")
	}

	// 损坏的 meta.json 只影响自身。
	broken := filepath.Join(RestorePointsDir(), "20200101_000000_000000")
	if err := os.MkdirAll(broken, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, RestorePointMetaName), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := len(ListRestorePoints(RestorePointFilter{})); got != 3 {
		t.Errorf("损坏条目应被跳过，得到 %d 条", got)
	}
}

// 回滚：先给当前状态留一个还原点，再把原内容写回；回滚“不存在”状态时移走当前文件。
func TestRestoreRestorePoint(t *testing.T) {
	home := restorePointHome(t)
	target := filepath.Join(home, ".zcode", "v2", "config.json")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"provider":{"a":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	point := createPoint(t, ChangeContext{Target: target, AgentID: string(AgentZCode), Operation: OpSaveProvider})

	if err := os.WriteFile(target, []byte(`{"provider":{"b":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	outcome, err := RestoreRestorePoint(point.ID)
	if err != nil {
		t.Fatalf("RestoreRestorePoint 失败：%v", err)
	}
	if outcome.Snapshot == nil || outcome.Snapshot.Operation != OpRestore {
		t.Error("回滚前应为当前状态建立 operation=restore 的还原点")
	}
	if got := mustReadFile(t, target); !strings.Contains(got, `"a"`) {
		t.Errorf("回滚后内容不正确：%s", got)
	}
	// 回滚前的那份状态也要能再回滚回去。
	back, err := RestoreRestorePoint(outcome.Snapshot.ID)
	if err != nil {
		t.Fatalf("二次回滚失败：%v", err)
	}
	if back.TargetPath != target {
		t.Errorf("二次回滚目标 = %q", back.TargetPath)
	}
	if got := mustReadFile(t, target); !strings.Contains(got, `"b"`) {
		t.Errorf("二次回滚后应回到回滚前的内容：%s", got)
	}
}

// “当时文件不存在”的回滚语义：当前文件被重命名成 .removed_<时间戳>，而不是直接删除。
func TestRestoreToAbsentState(t *testing.T) {
	home := restorePointHome(t)
	target := filepath.Join(home, ".zcode", "v2", "provider_config.json")
	point := createPoint(t, ChangeContext{Target: target, AgentID: string(AgentZCode), Operation: OpSaveProvider})
	if point.Existed {
		t.Fatal("前提：还原点记录的是文件不存在")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"schemaVersion":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	outcome, err := RestoreRestorePoint(point.ID)
	if err != nil {
		t.Fatalf("RestoreRestorePoint 失败：%v", err)
	}
	if outcome.RemovedPath == "" {
		t.Fatal("应返回被移走的文件路径")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("目标文件应已不存在")
	}
	if !strings.HasPrefix(filepath.Base(outcome.RemovedPath), filepath.Base(target)+".removed_") {
		t.Errorf("被移走的文件名不符合约定：%s", outcome.RemovedPath)
	}
	if got := mustReadFile(t, outcome.RemovedPath); !strings.Contains(got, "schemaVersion") {
		t.Errorf("被移走的是原文件内容：%s", got)
	}
}

// 内容被篡改时拒绝回滚。
func TestRestoreRejectsTamperedContent(t *testing.T) {
	home := restorePointHome(t)
	target := filepath.Join(home, ".zcode", "v2", "config.json")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"provider":{"a":{}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	point := createPoint(t, ChangeContext{Target: target, AgentID: string(AgentZCode), Operation: OpSaveProvider})
	contentPath, _ := RestorePointContentPath(point.ID)
	if err := os.WriteFile(contentPath, []byte(`{"provider":{"tampered":{}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreRestorePoint(point.ID); err == nil {
		t.Fatal("内容 sha256 不一致时必须拒绝回滚")
	}
}

// 保留策略：每个目标最多 MaxRestorePoints 个，超出的按最旧优先删除。
func TestPruneRestorePointsRetention(t *testing.T) {
	home := restorePointHome(t)
	target := filepath.Join(home, ".zcode", "v2", "config.json")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"provider":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	// 直接造出超量目录（比逐个创建快，且不依赖时间戳唯一性）。
	root := RestorePointsDir()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	base := MaxRestorePoints + 5
	for i := 0; i < base; i++ {
		dir := filepath.Join(root, idForIndex(i))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		meta := RestorePoint{
			ID:         idForIndex(i),
			CreatedAt:  "2026-01-01T00:00:00Z",
			AgentID:    string(AgentZCode),
			TargetPath: target,
			Format:     FormatLegacy,
			Existed:    true,
			SizeBytes:  10,
			Operation:  OpSaveProvider,
		}
		data, _ := json.Marshal(meta)
		if err := os.WriteFile(filepath.Join(dir, RestorePointMetaName), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// 再放一个没有 meta.json 的残留目录（应被孤儿清理逻辑忽略：太新）。
	orphan := filepath.Join(root, "20301231_235959_999999")
	if err := os.MkdirAll(orphan, 0700); err != nil {
		t.Fatal(err)
	}
	if removed := PruneRestorePoints(); removed != base-MaxRestorePoints {
		t.Fatalf("清理条数 = %d, want %d", removed, base-MaxRestorePoints)
	}
	if got := len(ListRestorePoints(RestorePointFilter{})); got != MaxRestorePoints {
		t.Fatalf("保留条数 = %d, want %d", got, MaxRestorePoints)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Error("过新的孤儿目录不应被清理")
	}
}

// idForIndex 造出稳定且可排序的还原点目录名。
func idForIndex(i int) string {
	return "2026010" + string(rune('0'+i/10)) + "_0000" + string(rune('0'+i%10)) + "0_000000"
}

// ID 校验必须挡住路径穿越。
func TestRestorePointIDValidation(t *testing.T) {
	for _, bad := range []string{"", "..", "../x", "20260101_000000_000000/../../x", "abc"} {
		if ValidRestorePointID(bad) {
			t.Errorf("ID %q 应被判为非法", bad)
		}
		if _, err := ReadRestorePoint(bad); err == nil {
			t.Errorf("ID %q 读取应报错", bad)
		}
	}
	if !ValidRestorePointID("20260101_000000_000000-1") {
		t.Error("带序号后缀的 ID 应合法")
	}
}
