package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 本文件是「DSH（YAML）的最近备份无法恢复」的回归测试：格式判定（isYAMLPath）必须
// 先剥离固定词表的辅助文件后缀（见 AGENTS.md「写入是原子的」），但绝不能把 .json
// 系列猜成 YAML。所有触盘用例一律 t.TempDir() + 占位值，绝不读写真实用户配置。

// writeRestoreFormatFile 在临时目录里写一个占位配置文件。
func writeRestoreFormatFile(t *testing.T, path, content string) {
	t.Helper()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("创建目录 %s 失败：%v", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入 %s 失败：%v", path, err)
	}
}

// readRestoreFormatFile 读取占位配置文件内容。
func readRestoreFormatFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	return string(data)
}

// restoreFormatSandbox 把用户目录指向临时目录，确保任何路径解析都落在沙箱内。
func restoreFormatSandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	return home
}

// TestIsYAMLPathStripsFixedVocabularySuffix 表驱动覆盖格式判定：
//   - .yaml/.yml 及其缓存、备份、快照、移除、临时后缀一律判成 YAML；
//   - .json 系列带上同样的后缀仍然判成 JSON；
//   - 未知后缀、非法时间戳不猜测。
func TestIsYAMLPathStripsFixedVocabularySuffix(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"普通 yaml 后缀", "settings.yaml", true},
		{"普通 yml 后缀", "settings.yml", true},
		{"大写后缀", "C:/Users/placeholder/.dsh/Settings.YAML", true},
		{"备份后缀", "settings.yaml.bak_20261018_101010_123456", true},
		{"快照后缀", "settings.yaml.snapshot_20261018_101010_123456", true},
		{"移除后缀", "settings.yaml.removed_20261018_101010_123456", true},
		{"移除后缀带冲突序号", "settings.yaml.removed_20261018_101010_2", true},
		{"临时文件后缀", "settings.yaml.tmp_123456789", true},
		{"恢复中间文件后缀", "settings.yaml.restore_987654321", true},
		{"更新器暂存后缀", "x.yaml.apm-update-123", true},
		{"更新器备份后缀", "x.yaml.apm-backup-456", true},
		{"连续多重后缀", "settings.yaml.bak_20261018_101010_123456.tmp_987654321", true},
		{"后缀整体大小写不敏感", "settings.YAML.BAK_20261018_101010_123456", true},
		{"目录名含 .tmp_ 不误剥", "C:/x.tmp_y/settings.yaml", true},
		{"普通 json 后缀", "providers.json", false},
		{"json 备份后缀", "providers.json.bak_20261018_101010_123456", false},
		{"json 快照后缀", "providers.json.snapshot_20261018_101010_123456", false},
		{"json 更新器暂存后缀", "providers.json.apm-update-123", false},
		{"json 连续多重后缀", "providers.json.bak_20261018_101010_123456.tmp_987654321", false},
		{"无后缀", "settings", false},
		{"空路径", "", false},
		{"备份后缀缺时间戳", "settings.yaml.bak_", false},
		{"备份后缀时间戳分段不足", "settings.yaml.bak_20261018_101010", false},
		{"临时后缀缺尾巴", "settings.yaml.tmp_", false},
		{"未知后缀不猜测", "settings.yaml.baknot", false},
		{"未知词表后缀不猜测", "settings.yaml.weird_20261018_101010_123456", false},
		{"更新器 exe 尾巴不是配置名", "x.yaml.apm-update-1234.exe", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := isYAMLPath(tc.path); got != tc.want {
				t.Errorf("isYAMLPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestStripAuxSuffixKeepsUnknownSuffix 直接锁住「保守剥离」的契约：
// 已知词表后缀被逐层剥掉，未知后缀原样保留。
func TestStripAuxSuffixKeepsUnknownSuffix(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"剥离备份后缀", "settings.yaml.bak_20261018_101010_123456", "settings.yaml"},
		{"剥离连续多重后缀", "settings.yaml.bak_20261018_101010_123456.tmp_987654321", "settings.yaml"},
		{"不剥离未知后缀", "settings.yaml.weird_20261018_101010_123456", "settings.yaml.weird_20261018_101010_123456"},
		{"不剥离普通路径", "settings.yaml", "settings.yaml"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := stripAuxSuffix(tc.path); got != tc.want {
				t.Errorf("stripAuxSuffix(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestLoadConfigForRestoreParsesDSHYAMLBackup 是缺陷的最小复现：
// settings.yaml.bak_<ts> 带 YAML 专属语法（不是合法 JSON），修复前会被当成 JSON
// 走 LoadConfig 而必然失败，修复后必须与 settings.yaml 走同一条 YAML 分支。
func TestLoadConfigForRestoreParsesDSHYAMLBackup(t *testing.T) {
	home := restoreFormatSandbox(t)
	backup := filepath.Join(home, ".dsh", "settings.yaml.bak_20261018_101010_123456")
	writeRestoreFormatFile(t, backup, "agent-default-model: acme\nproviders:\n  acme:\n    apiKey: sk-placeholder\n")

	cfg, err := LoadConfigForRestore(backup)
	if err != nil {
		t.Fatalf("LoadConfigForRestore 解析 YAML 备份失败：%v", err)
	}
	if got, _ := cfg["agent-default-model"].(string); got != "acme" {
		t.Errorf("agent-default-model = %q, want %q", got, "acme")
	}
	providers, ok := cfg["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("providers 段类型不正确：%T", cfg["providers"])
	}
	acme, ok := providers["acme"].(map[string]interface{})
	if !ok {
		t.Fatalf("providers.acme 段类型不正确：%T", providers["acme"])
	}
	if got, _ := acme["apiKey"].(string); got != "sk-placeholder" {
		t.Errorf("apiKey = %q, want %q", got, "sk-placeholder")
	}
}

// TestRestoreBackupRestoresYAMLBackup 是端到端用例：RestoreBackup 对 DSH 的
// settings.yaml.bak_<ts> 必须能解析并写回目标文件，并把恢复前的内容留在
// .snapshot_<ts> 快照里。修复前该调用会在 LoadConfigForRestore 处直接失败。
func TestRestoreBackupRestoresYAMLBackup(t *testing.T) {
	home := restoreFormatSandbox(t)
	target := filepath.Join(home, ".dsh", "settings.yaml")
	current := "agent-default-model: current\nproviders: {}\n"
	previous := "agent-default-model: previous\nproviders:\n  acme:\n    apiKey: sk-placeholder\n"
	writeRestoreFormatFile(t, target, current)
	backup := target + ".bak_20261018_101010_123456"
	writeRestoreFormatFile(t, backup, previous)

	snap, err := RestoreBackup(target, backup)
	if err != nil {
		t.Fatalf("RestoreBackup 恢复 DSH YAML 备份失败：%v", err)
	}
	if got := readRestoreFormatFile(t, target); got != previous {
		t.Errorf("目标文件内容未被还原：\n got = %q\nwant = %q", got, previous)
	}
	if snap == "" {
		t.Fatal("应返回恢复前的 .snapshot_ 快照路径")
	}
	if !strings.HasPrefix(filepath.Base(snap), "settings.yaml.snapshot_") {
		t.Errorf("快照文件名不符合约定：%s", snap)
	}
	if got := readRestoreFormatFile(t, snap); got != current {
		t.Errorf("快照内容不是恢复前的原文：\n got = %q\nwant = %q", got, current)
	}
}

// TestRestoreBackupKeepsJSONBackupAsJSON 保证 .json 系列备份不被误判成 YAML：
// 备份内容原样写回，且 LoadConfigForRestore 仍按 JSON 解析出 provider 段。
func TestRestoreBackupKeepsJSONBackupAsJSON(t *testing.T) {
	home := restoreFormatSandbox(t)
	target := filepath.Join(home, ".zcode", "config.json")
	current := "{\n  // 备份前的当前配置\n  \"provider\": {}\n}\n"
	previous := "{\"provider\":{\"acme\":{\"apiKey\":\"sk-placeholder\"}}}\n"
	writeRestoreFormatFile(t, target, current)
	backup := target + ".bak_20261018_101010_123456"
	writeRestoreFormatFile(t, backup, previous)

	if isYAMLPath(backup) {
		t.Fatalf("JSON 备份被误判成 YAML：%s", backup)
	}
	cfg, err := LoadConfigForRestore(backup)
	if err != nil {
		t.Fatalf("LoadConfigForRestore 解析 JSON 备份失败：%v", err)
	}
	if _, ok := cfg["provider"].(map[string]interface{}); !ok {
		t.Fatalf("provider 段类型不正确：%T", cfg["provider"])
	}

	snap, err := RestoreBackup(target, backup)
	if err != nil {
		t.Fatalf("RestoreBackup 恢复 JSON 备份失败：%v", err)
	}
	if got := readRestoreFormatFile(t, target); got != previous {
		t.Errorf("目标文件内容未被还原：\n got = %q\nwant = %q", got, previous)
	}
	if snap == "" {
		t.Fatal("应返回恢复前的 .snapshot_ 快照路径")
	}
}
