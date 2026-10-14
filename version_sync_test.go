package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
)

// 版本号唯一真源是仓库根目录的 VERSION 文件，scripts/sync-version.mjs 把它
// 分发到各消费点。本测试在 `go test ./...` 中兜底：任何派生文件与真源不一致
// 就失败，避免忘记运行同步脚本导致版本号静默漂移。修复方式见失败信息。
func TestVersionSourcesInSync(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatalf("读取 VERSION 失败: %v", err)
	}
	version := strings.TrimSpace(string(raw))
	if version == "" {
		t.Fatal("VERSION 文件为空")
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(version) {
		t.Fatalf("VERSION 不是合法的语义化版本号: %q", version)
	}

	fail := func(format string, args ...interface{}) {
		t.Errorf(format+"\n修复：编辑 VERSION 后运行 node scripts/sync-version.mjs", args...)
	}

	// Go 常量（前端与 --version 都最终读它）
	if core.AppVersion != version {
		fail("core.AppVersion = %q, 期望 %q", core.AppVersion, version)
	}

	// internal/core/version_gen.go 由脚本生成，必须与真源一致
	gen, err := os.ReadFile(filepath.Join(root, "internal", "core", "version_gen.go"))
	if err != nil {
		fail("读取 internal/core/version_gen.go 失败: %v", err)
	} else if !strings.Contains(string(gen), fmt.Sprintf("const AppVersion = %q", version)) {
		fail("internal/core/version_gen.go 未反映 VERSION=%q", version)
	}

	// wails.json → Windows PE 版本信息
	assertJSONVersion(t, root, "wails.json", "productVersion", version, fail)
	// frontend/package.json
	assertJSONVersion(t, root, filepath.Join("frontend", "package.json"), "version", version, fail)

	// package.json.md5 必须是 package.json 的字节级 MD5（Wails 用它判断是否重装依赖）
	pkg, err := os.ReadFile(filepath.Join(root, "frontend", "package.json"))
	if err != nil {
		fail("读取 frontend/package.json 失败: %v", err)
	} else {
		want := fmt.Sprintf("%x", md5.Sum(pkg))
		md5Path := filepath.Join(root, "frontend", "package.json.md5")
		got, err := os.ReadFile(md5Path)
		if err != nil {
			fail("读取 frontend/package.json.md5 失败: %v", err)
		} else if strings.TrimSpace(string(got)) != want {
			fail("frontend/package.json.md5 = %q, 期望 %q", strings.TrimSpace(string(got)), want)
		}
	}

	// frontend/package-lock.json 的根包两处 version
	lockRaw, err := os.ReadFile(filepath.Join(root, "frontend", "package-lock.json"))
	if err != nil {
		fail("读取 frontend/package-lock.json 失败: %v", err)
	} else {
		var lock struct {
			Version  string `json:"version"`
			Packages map[string]struct {
				Version string `json:"version"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(lockRaw, &lock); err != nil {
			fail("解析 frontend/package-lock.json 失败: %v", err)
		} else {
			if lock.Version != version {
				fail("package-lock.json 顶层 version = %q, 期望 %q", lock.Version, version)
			}
			if pkgRoot, ok := lock.Packages[""]; ok && pkgRoot.Version != version {
				fail(`package-lock.json packages[""].version = %q, 期望 %q`, pkgRoot.Version, version)
			}
		}
	}

	// README.md：标题与下载文件名
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		fail("读取 README.md 失败: %v", err)
	} else {
		text := string(readme)
		if !strings.Contains(text, "# Agent Provider Manager "+version) {
			fail("README.md 标题未反映版本 %q", version)
		}
		asset := "AgentProviderManager-" + version + "-windows-amd64"
		if !strings.Contains(text, asset) {
			fail("README.md 未包含下载文件名 %q", asset)
		}
	}

	// 前端不得再硬编码版本号（侧栏改为运行时从 GetAppInfo 读取）
	sidebar, err := os.ReadFile(filepath.Join(root, "frontend", "src", "layout", "AppSidebar.vue"))
	if err != nil {
		fail("读取 AppSidebar.vue 失败: %v", err)
	} else if regexp.MustCompile(`ref\('[0-9]+\.[0-9]+\.[0-9]+`).Match(sidebar) {
		fail("frontend/src/layout/AppSidebar.vue 仍硬编码版本号，应改为运行时读取 GetAppInfo")
	}
}

func assertJSONVersion(t *testing.T, root, rel, key, want string, fail func(string, ...interface{})) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		fail("读取 %s 失败: %v", rel, err)
		return
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		fail("解析 %s 失败: %v", rel, err)
		return
	}
	if key == "productVersion" {
		if info, ok := obj["info"].(map[string]interface{}); ok {
			if got, _ := info[key].(string); got != want {
				fail("%s info.%s = %q, 期望 %q", rel, key, got, want)
			}
			return
		}
		fail("%s 缺少 info.%s", rel, key)
		return
	}
	if got, _ := obj[key].(string); got != want {
		fail("%s %s = %q, 期望 %q", rel, key, got, want)
	}
}
