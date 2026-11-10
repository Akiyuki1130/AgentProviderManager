package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// omp 还原点：models.yml 是 provider 目标，config.yml 只承载 modelRoles 等设置（G7）。
// 两者只做 YAML + 根 mapping 的宽松校验，且 config.yml 回滚不得改变 provider 目标文件。

func TestConfigFormatAtOMP(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"C:/Users/x/.omp/agent/models.yml", FormatOMP},
		{"C:/Users/x/.omp/agent/models.yaml", FormatOMP},
		{"C:/Users/x/.omp/agent/models.json", FormatOMP},
		{"C:/Users/x/.omp/agent/config.yml", FormatOMPConfig},
		{"C:/Users/x/.omp/agent/config.yaml", FormatOMPConfig},
		// 备份/还原辅助后缀必须先剥离再判定，否则历史还原点会被误判成 models 格式。
		{"C:/Users/x/.omp/agent/config.yml.bak_20240101_000000_000000", FormatOMPConfig},
		{"C:/Users/x/.omp/agent/models.json.bak_20240101_000000_000000", FormatOMP},
	}
	for _, tc := range cases {
		if got := ConfigFormatAt(tc.path, "omp"); got != tc.want {
			t.Errorf("ConfigFormatAt(%q, omp) = %q, want %q", tc.path, got, tc.want)
		}
	}
	// 其它 harness 的判定不受影响：DSH 的 YAML 仍然是 deepseek。
	if got := ConfigFormatAt("C:/Users/x/.dsh/settings.yaml", "deepseek"); got != FormatDeepSeek {
		t.Errorf("deepseek yaml = %q", got)
	}
}

func TestCreateRestorePointOMPInfersFormat(t *testing.T) {
	home := restorePointHome(t)
	target := filepath.Join(home, ".omp", "agent", "config.yml")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("modelRoles:\n  default: acme/m1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	point := createPoint(t, ChangeContext{Target: target, AgentID: "omp", Operation: OpSetDefaultModel})
	if point.Format != FormatOMPConfig {
		t.Fatalf("point format = %q, want %q", point.Format, FormatOMPConfig)
	}
	if point.Operation != OpSetDefaultModel {
		t.Errorf("operation = %q", point.Operation)
	}
}

func TestValidateRestoreContentOMP(t *testing.T) {
	cases := []struct {
		name      string
		format    string
		content   string
		want      string // 期望的提示子串；空串表示应当通过
		wantFatal bool   // true 表示必须是硬错误（调用方不得写盘）
	}{
		{"models yaml ok", FormatOMP, "providers:\n  acme: {}\n", "", false},
		{"models empty ok", FormatOMP, "", "", false},
		{"models根不是 mapping", FormatOMP, "- one\n- two\n", "YAML", false},
		{"models 非法 YAML", FormatOMP, "providers: [unclosed\n", "YAML", false},
		{"config yaml ok", FormatOMPConfig, "modelRoles:\n  default: acme/m1\n", "", false},
		{"config 不含 providers 也允许", FormatOMPConfig, "modelRoles:\n  fast: acme/m2\n", "", false},
		// omp 读不了的设置必须中止回滚，否则会把用户配置写坏。
		{"config modelRoles 非 mapping", FormatOMPConfig, "modelRoles:\n  - acme/m1\n", "modelRoles", true},
		{"config 非法 YAML", FormatOMPConfig, "modelRoles: {\n", "YAML", false},
	}
	for _, tc := range cases {
		point := &RestorePoint{Format: tc.format, TargetPath: "C:/Users/x/.omp/agent/models.yml"}
		warning, fatal := validateRestoreContent([]byte(tc.content), point)
		if tc.want == "" {
			if warning != "" || fatal != nil {
				t.Errorf("%s: unexpected warning %q / fatal %v", tc.name, warning, fatal)
			}
			continue
		}
		if tc.wantFatal {
			if fatal == nil {
				t.Errorf("%s: 期望硬错误，得到 warning %q", tc.name, warning)
				continue
			}
			if !strings.Contains(fatal.Error(), tc.want) {
				t.Errorf("%s: fatal = %q, want substring %q", tc.name, fatal, tc.want)
			}
			continue
		}
		if fatal != nil {
			t.Errorf("%s: 不应是硬错误：%v", tc.name, fatal)
			continue
		}
		if !strings.Contains(warning, tc.want) {
			t.Errorf("%s: warning = %q, want substring %q", tc.name, warning, tc.want)
		}
	}
}

func TestRestoreOMPConfigWritesContent(t *testing.T) {
	home := restorePointHome(t)
	dir := filepath.Join(home, ".omp", "agent")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	models := filepath.Join(dir, "models.yml")
	config := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(models, []byte("providers:\n  acme:\n    baseUrl: https://example.invalid/v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("modelRoles:\n  default: acme/m1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	point := createPoint(t, ChangeContext{Target: config, AgentID: "omp", Operation: OpSetDefaultModel})
	if point.Format != FormatOMPConfig || !point.Existed {
		t.Fatalf("point = %+v", point)
	}
	// 模拟 omp 侧改过设置文件，再回滚：内容必须恢复，且 provider 目标文件不受影响。
	if err := os.WriteFile(config, []byte("modelRoles:\n  default: other/m2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	outcome, err := RestoreRestorePoint(point.ID)
	if err != nil {
		t.Fatalf("RestoreRestorePoint: %v", err)
	}
	if outcome.TargetPath != config {
		t.Errorf("target path = %q, want %q", outcome.TargetPath, config)
	}
	if got := mustReadFile(t, config); got != "modelRoles:\n  default: acme/m1\n" {
		t.Errorf("config content = %q", got)
	}
	if got := mustReadFile(t, models); !strings.Contains(got, "baseUrl") {
		t.Errorf("models file touched unexpectedly: %q", got)
	}
	// 回滚前快照记录的是被替换掉的那一版设置文件，格式也必须是 omp-config。
	if outcome.Snapshot == nil || outcome.Snapshot.Format != FormatOMPConfig {
		t.Fatalf("snapshot = %+v", outcome.Snapshot)
	}
}

// TestRestoreOMPConfigInvalidModelRolesDoesNotWrite 覆盖「历史还原点里出现 omp 读不了的设置」：
// 硬校验失败时回滚必须整体失败，且目标文件一个字节都不能被改。
func TestRestoreOMPConfigInvalidModelRolesDoesNotWrite(t *testing.T) {
	home := restorePointHome(t)
	dir := filepath.Join(home, ".omp", "agent")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "config.yml")
	const original = "modelRoles:\n  default: acme/m1\n"
	if err := os.WriteFile(config, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	point := createPoint(t, ChangeContext{Target: config, AgentID: "omp", Operation: OpSetDefaultModel})
	if point.Format != FormatOMPConfig {
		t.Fatalf("point format = %q, want %q", point.Format, FormatOMPConfig)
	}
	// 把还原点内容替换成 omp 读不了的 modelRoles，并同步 sha256，模拟历史还原点。
	bad := []byte("modelRoles:\n  - acme/m1\n")
	contentPath, err := RestorePointContentPath(point.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contentPath, bad, 0600); err != nil {
		t.Fatal(err)
	}
	point.SHA256 = sha256Hex(bad)
	meta, err := json.MarshalIndent(point, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(RestorePointsDir(), point.ID, RestorePointMetaName), meta, 0600); err != nil {
		t.Fatal(err)
	}
	// 用户侧在建立还原点后继续改过设置文件，回滚失败时必须保持这一版原样。
	const current = "modelRoles:\n  default: other/m2\n"
	if err := os.WriteFile(config, []byte(current), 0644); err != nil {
		t.Fatal(err)
	}
	outcome, err := RestoreRestorePoint(point.ID)
	if err == nil || !strings.Contains(err.Error(), "modelRoles") {
		t.Fatalf("RestoreRestorePoint err = %v，期望 modelRoles 硬错误", err)
	}
	if outcome.Warning != "" {
		t.Errorf("warning = %q，硬错误不应带非致命提示", outcome.Warning)
	}
	if got := mustReadFile(t, config); got != current {
		t.Errorf("硬校验失败时不得写盘：config content = %q", got)
	}
}
