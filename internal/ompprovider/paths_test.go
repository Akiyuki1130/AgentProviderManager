package ompprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandboxHome 把 USERPROFILE 与 PI_CONFIG_DIR 指向临时目录。
func sandboxHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("创建临时 home 失败：%v", err)
	}
	t.Setenv("USERPROFILE", home)
	t.Setenv("PI_CONFIG_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("OMP_PROFILE", "")
	t.Setenv("PI_PROFILE", "")
	return home
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
}

func TestHomeAndProfile(t *testing.T) {
	home := sandboxHome(t)
	if got := Home(); got != filepath.Join(home, ".omp") {
		t.Fatalf("Home=%q，期望 %q", got, filepath.Join(home, ".omp"))
	}

	configDir := filepath.Join(t.TempDir(), "omp-config")
	t.Setenv("PI_CONFIG_DIR", configDir)
	if got := Home(); got != configDir {
		t.Fatalf("PI_CONFIG_DIR 应优先：%q", got)
	}

	t.Setenv("OMP_PROFILE", "work")
	if got := ActiveProfile(); got != "work" {
		t.Fatalf("OMP_PROFILE 应优先：%q", got)
	}
	os.Unsetenv("OMP_PROFILE")
	t.Setenv("PI_PROFILE", "legacy")
	if got := ActiveProfile(); got != "legacy" {
		t.Fatalf("PI_PROFILE 应生效：%q", got)
	}
	t.Setenv("PI_PROFILE", "")
	if got := ActiveProfile(); got != "" {
		t.Fatalf("无 profile 变量时应返回空串：%q", got)
	}
}

func TestAgentDir(t *testing.T) {
	home := sandboxHome(t)
	ompHome := filepath.Join(home, ".omp")

	if got, want := AgentDir(""), filepath.Join(ompHome, "agent"); got != want {
		t.Fatalf("默认 profile：%q，期望 %q", got, want)
	}
	override := filepath.Join(t.TempDir(), "custom-agent")
	t.Setenv("PI_CODING_AGENT_DIR", override)
	if got := AgentDir(""); got != override {
		t.Fatalf("PI_CODING_AGENT_DIR 应覆盖默认 profile：%q", got)
	}
	if got, want := AgentDir("work"), filepath.Join(ompHome, "profiles", "work", "agent"); got != want {
		t.Fatalf("命名 profile 应忽略 PI_CODING_AGENT_DIR：%q，期望 %q", got, want)
	}
	if got := BaseDir(); got != override {
		t.Fatalf("BaseDir=%q，期望 %q", got, override)
	}
}

func TestModelsPathAndWritePath(t *testing.T) {
	home := sandboxHome(t)
	agent := filepath.Join(home, ".omp", "agent")
	writeFile(t, filepath.Join(agent, "models.json"), "{}")
	writeFile(t, filepath.Join(agent, "models.yaml"), "providers: {}\n")

	t.Run("只有 json 时需要迁移且不可写", func(t *testing.T) {
		os.Remove(filepath.Join(agent, "models.yaml"))
		path, needsMigration := ModelsPath("")
		if path != filepath.Join(agent, "models.json") || !needsMigration {
			t.Fatalf("ModelsPath=%q migration=%v", path, needsMigration)
		}
		if writePath, writable := ModelsWritePath(""); writePath != "" || writable {
			t.Fatalf("只有 json 时不可写：%q %v", writePath, writable)
		}
	})
	t.Run("yaml 优先于 json", func(t *testing.T) {
		writeFile(t, filepath.Join(agent, "models.yaml"), "providers: {}\n")
		path, needsMigration := ModelsPath("")
		if path != filepath.Join(agent, "models.yaml") || needsMigration {
			t.Fatalf("ModelsPath=%q migration=%v", path, needsMigration)
		}
		if writePath, writable := ModelsWritePath(""); writePath != filepath.Join(agent, "models.yaml") || !writable {
			t.Fatalf("ModelsWritePath=%q writable=%v", writePath, writable)
		}
	})
	t.Run("yml 优先于 yaml", func(t *testing.T) {
		writeFile(t, filepath.Join(agent, "models.yml"), "providers: {}\n")
		path, _ := ModelsPath("")
		if path != filepath.Join(agent, "models.yml") {
			t.Fatalf("yml 应优先：%q", path)
		}
		if writePath, writable := ModelsWritePath(""); writePath != filepath.Join(agent, "models.yml") || !writable {
			t.Fatalf("ModelsWritePath=%q writable=%v", writePath, writable)
		}
	})
	t.Run("都没有时返回新 yml", func(t *testing.T) {
		for _, name := range []string{"models.yml", "models.yaml", "models.json"} {
			os.Remove(filepath.Join(agent, name))
		}
		path, needsMigration := ModelsPath("")
		if path != filepath.Join(agent, "models.yml") || needsMigration {
			t.Fatalf("ModelsPath=%q migration=%v", path, needsMigration)
		}
		if writePath, writable := ModelsWritePath(""); writePath != filepath.Join(agent, "models.yml") || !writable {
			t.Fatalf("ModelsWritePath=%q writable=%v", writePath, writable)
		}
	})
}

func TestConfigPath(t *testing.T) {
	home := sandboxHome(t)
	agent := filepath.Join(home, ".omp", "agent")

	path, exists := ConfigPath("")
	if path != filepath.Join(agent, "config.yml") || exists {
		t.Fatalf("缺失时应返回新 config.yml：%q %v", path, exists)
	}
	writeFile(t, filepath.Join(agent, "config.yaml"), "modelRoles: {}\n")
	path, exists = ConfigPath("")
	if path != filepath.Join(agent, "config.yaml") || !exists {
		t.Fatalf("应识别 config.yaml：%q %v", path, exists)
	}
	writeFile(t, filepath.Join(agent, "config.yml"), "modelRoles: {}\n")
	path, exists = ConfigPath("")
	if path != filepath.Join(agent, "config.yml") || !exists {
		t.Fatalf("config.yml 应优先：%q %v", path, exists)
	}
}

func TestProfiles(t *testing.T) {
	home := sandboxHome(t)
	base := filepath.Join(home, ".omp", "profiles")
	for _, name := range []string{"zeta", "alpha"} {
		if err := os.MkdirAll(filepath.Join(base, name, "agent"), 0o755); err != nil {
			t.Fatalf("创建 profile 失败：%v", err)
		}
	}
	// 缺少 agent 子目录的条目不算 profile。
	if err := os.MkdirAll(filepath.Join(base, "broken"), 0o755); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	got := Profiles()
	if strings.Join(got, ",") != "alpha,zeta" {
		t.Fatalf("Profiles=%v", got)
	}
}

func TestIsUnderHome(t *testing.T) {
	home := sandboxHome(t)
	ompHome := filepath.Join(home, ".omp")

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "home 本身", path: ompHome, want: true},
		{name: "home 子路径", path: filepath.Join(ompHome, "agent", "models.yml"), want: true},
		{name: "相对路径解析后仍在 home", path: filepath.Join(ompHome, "..", ".omp", "agent"), want: true},
		{name: "home 外", path: filepath.Join(home, "other", "models.yml"), want: false},
		{name: "空字符串", path: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsUnderHome(test.path); got != test.want {
				t.Fatalf("IsUnderHome(%q)=%v，期望 %v", test.path, got, test.want)
			}
		})
	}
}

func TestIsConfigFileName(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "models.yml", want: true},
		{path: "models.yaml", want: true},
		{path: "models.json", want: true},
		{path: "config.yml", want: true},
		{path: "config.yaml", want: true},
		{path: "models.yml.bak_20260101", want: true},
		{path: "config.yaml.tmp_abc", want: true},
		{path: "settings.json", want: false},
		{path: "other.yml", want: false},
	}
	for _, test := range tests {
		if got := IsConfigFileName(test.path); got != test.want {
			t.Fatalf("IsConfigFileName(%q)=%v，期望 %v", test.path, got, test.want)
		}
	}
}
