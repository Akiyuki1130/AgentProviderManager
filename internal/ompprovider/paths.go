package ompprovider

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultConfigDirName 是 omp 配置根目录的默认名。
const defaultConfigDirName = ".omp"

// Home 返回 omp 配置根目录：%PI_CONFIG_DIR% 或 %USERPROFILE%\.omp。
func Home() string {
	dir := strings.TrimSpace(os.Getenv("PI_CONFIG_DIR"))
	if dir == "" {
		dir = defaultConfigDirName
	}
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	base := userHomeDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, dir)
}

// ActiveProfile 返回当前活跃 profile；空字符串表示默认 profile。
func ActiveProfile() string {
	for _, key := range []string{"OMP_PROFILE", "PI_PROFILE"} {
		if raw, ok := os.LookupEnv(key); ok {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

// AgentDir 返回指定 profile 的 agent 目录。
func AgentDir(profile string) string {
	name := strings.TrimSpace(profile)
	if name != "" {
		home := Home()
		if home == "" {
			return ""
		}
		return filepath.Join(home, "profiles", name, "agent")
	}
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	home := Home()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "agent")
}

// BaseDir 返回活跃 profile 的 agent 目录。
func BaseDir() string { return AgentDir(ActiveProfile()) }

// ModelsPath 返回 models 文件的读取路径；needsMigration 表示当前只有 models.json。
func ModelsPath(profile string) (string, bool) {
	dir := AgentDir(profile)
	if dir == "" {
		return "", false
	}
	for _, name := range []string{"models.yml", "models.yaml"} {
		p := filepath.Join(dir, name)
		if fileExists(p) {
			return p, false
		}
	}
	jsonPath := filepath.Join(dir, "models.json")
	if fileExists(jsonPath) {
		return jsonPath, true
	}
	return filepath.Join(dir, "models.yml"), false
}

// ModelsWritePath 返回可写路径；只有 models.json 时返回空串与 false（G1：P1 不新建 .yml）。
func ModelsWritePath(profile string) (string, bool) {
	dir := AgentDir(profile)
	if dir == "" {
		return "", false
	}
	if p := filepath.Join(dir, "models.yml"); fileExists(p) {
		return p, true
	}
	if p := filepath.Join(dir, "models.yaml"); fileExists(p) {
		return p, true
	}
	if fileExists(filepath.Join(dir, "models.json")) {
		return "", false
	}
	return filepath.Join(dir, "models.yml"), true
}

// ConfigPath 返回 config.yml 路径；exists 表示文件已存在。
func ConfigPath(profile string) (string, bool) {
	dir := AgentDir(profile)
	if dir == "" {
		return "", false
	}
	if p := filepath.Join(dir, "config.yml"); fileExists(p) {
		return p, true
	}
	if p := filepath.Join(dir, "config.yaml"); fileExists(p) {
		return p, true
	}
	return filepath.Join(dir, "config.yml"), false
}

// Profiles 枚举 Home/profiles/*/agent 下的命名 profile，按名称排序。
func Profiles() []string {
	home := Home()
	if home == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(home, "profiles"))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentDir := filepath.Join(home, "profiles", entry.Name(), "agent")
		info, statErr := os.Stat(agentDir)
		if statErr != nil || !info.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

// IsUnderHome 判断路径是否位于 omp 配置根目录下。
func IsUnderHome(path string) bool {
	home := Home()
	if home == "" || strings.TrimSpace(path) == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(home, abs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

// IsConfigFileName 判断文件名是否属于 omp 的 models / config 配置。
func IsConfigFileName(path string) bool {
	switch strings.ToLower(stripDecoration(filepath.Base(path))) {
	case "models.yml", "models.yaml", "models.json", "config.yml", "config.yaml":
		return true
	}
	return false
}

func userHomeDir() string {
	if home := strings.TrimSpace(os.Getenv("USERPROFILE")); home != "" {
		return filepath.Clean(home)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Clean(home)
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
