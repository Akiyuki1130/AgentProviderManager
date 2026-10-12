// Package dshprovider 处理 DeepSeek Harness（DSH）的配置文档。
//
// DSH 0.2（桌面端与 CLI 的 web profile 都是这一套）把用户配置从单一
// settings.yaml 改成 profile 目录下的 patch 层：
//
//	$DSH_HOME/profiles/<name>/cordis.patch.yml   每个 profile 自己的用户 patch
//	$DSH_HOME/cordis.patch.yml                   home 级 patch（优先级更高）
//
// provider 由 patch 里 id 为 llm-pi-ai 的条目（插件
// @deepseek-ai/dsh-llm-pi-ai）的 config.providers 描述；凭据继续放在
// $DSH_HOME/.credentials.yaml，但 0.2 要求 version: 1 + refs/records 分节。
//
// 旧版（0.1）的 settings.yaml 只作为一次性导入来源保留，见 core 的 legacy backend。
package dshprovider

import (
	"os"
	"path/filepath"
	"strings"
)

// Format 是 DSH 配置目标的形态。
type Format string

const (
	// FormatUnknown 表示无法判定（由调用方回退到 legacy 实现）。
	FormatUnknown Format = "unknown"
	// FormatProfileDesktop 是桌面端 profile 的 cordis.patch.yml。
	FormatProfileDesktop Format = "profile-desktop"
	// FormatProfileWeb 是 CLI/浏览器 profile 的 cordis.patch.yml。
	FormatProfileWeb Format = "profile-web"
	// FormatProfileOther 是自定义 profile（dsh --profile <name>）的 cordis.patch.yml。
	FormatProfileOther Format = "profile-other"
	// FormatSettingsLegacy 是 0.1 的 settings.yaml / settings.yml / settings.json。
	FormatSettingsLegacy Format = "settings-legacy"
)

const (
	// ProfilesDirName 是 $DSH_HOME 下存放所有 profile 的目录。
	ProfilesDirName = "profiles"
	// ProfilePatchFilename 是 profile 目录内的用户 patch 层文件名。
	ProfilePatchFilename = "cordis.patch.yml"
	// HomePatchFilename 是 $DSH_HOME 根下的 tweak 层文件名。
	HomePatchFilename = "cordis.patch.yml"
	// DesktopProfileName 是 Electron 桌面端保留的 profile 名。
	DesktopProfileName = "desktop"
	// WebProfileName 是浏览器/CLI profile 的模板名。
	WebProfileName = "web"
	// LLMPiAIEntryID 是承载 provider 路由的 profile 条目 id。
	LLMPiAIEntryID = "llm-pi-ai"
	// LLMPiAIPluginName 是该条目的插件包名。
	LLMPiAIPluginName = "@deepseek-ai/dsh-llm-pi-ai"
	// AgentDefaultModelEntryID 是默认模型条目的 id。
	AgentDefaultModelEntryID = "agent-default-model"
	// CredentialsFilename 是 harness home 下的凭据文件名。
	CredentialsFilename = ".credentials.yaml"
)

// ProfileDir 返回 $DSH_HOME/profiles/<name>。
func ProfileDir(home, name string) string {
	return filepath.Join(home, ProfilesDirName, name)
}

// ProfilePatchPath 返回某个 profile 的用户 patch 路径。
func ProfilePatchPath(home, name string) string {
	return filepath.Join(ProfileDir(home, name), ProfilePatchFilename)
}

// HomePatchPath 返回 home 级 patch 路径。
func HomePatchPath(home string) string {
	return filepath.Join(home, HomePatchFilename)
}

// CredentialsPath 返回 harness home 下的凭据文件路径。
func CredentialsPath(home string) string {
	return filepath.Join(home, CredentialsFilename)
}

// IsProfilePatchPath 判断路径是否是某个 profile 下的 cordis.patch.yml。
func IsProfilePatchPath(path string) bool {
	return strings.EqualFold(filepath.Base(path), ProfilePatchFilename)
}

// ProfileNameAt 从路径中解析 profile 名；不是 <...>/profiles/<name>/cordis.patch.yml 时返回空串。
func ProfileNameAt(path string) string {
	if !IsProfilePatchPath(path) {
		return ""
	}
	dir := filepath.Dir(path)
	name := filepath.Base(dir)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return ""
	}
	if !strings.EqualFold(filepath.Base(filepath.Dir(dir)), ProfilesDirName) {
		// <...>/profiles/<name>/cordis.patch.yml 之外的同名文件不当作 profile patch。
		return ""
	}
	return name
}

// ProfileFormat 按 profile 名给出对应格式。
func ProfileFormat(name string) Format {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case DesktopProfileName:
		return FormatProfileDesktop
	case WebProfileName:
		return FormatProfileWeb
	default:
		return FormatProfileOther
	}
}

// Detect 依据路径与文件内容判定格式。
//
// 判定顺序：
//  1. cordis.patch.yml：profiles/<name>/ 下按 profile 名分 desktop/web/other；
//     其它位置（如 home 级 patch）在内容是数组或空/只有注释时按 profile patch；
//  2. settings.yaml/.yml/.json 且内容是可读的映射（或为空）-> legacy；
//  3. 内容是顶层数组且含 llm-pi-ai 条目 -> profile patch；
//  4. 其余为 unknown。
func Detect(path string, data []byte) Format {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case ProfilePatchFilename:
		if name := ProfileNameAt(path); name != "" {
			return ProfileFormat(name)
		}
		// home 级 patch（$DSH_HOME/cordis.patch.yml）不在 profiles/ 目录下，
		// ProfileNameAt 返回空。空文件、只有注释的文件与顶层数组都是 patch：
		// 若在这里回退成 unknown，就会被当成 0.1 settings，用 settings 布局覆写，
		// 让 DSH 再也读不出这份 patch。
		if isPatchShapedOrEmpty(data) {
			return FormatProfileOther
		}
	case "settings.yaml", "settings.yml", "settings.json":
		if !looksLikePatchSequence(data) {
			return FormatSettingsLegacy
		}
	}
	if looksLikePatchSequence(data) {
		return FormatProfileOther
	}
	return FormatUnknown
}

// DetectFile 读取文件内容后调用 Detect；文件不存在或读取失败时只按路径判定。
func DetectFile(path string) Format {
	data, err := os.ReadFile(path)
	if err != nil {
		return Detect(path, nil)
	}
	return Detect(path, data)
}
