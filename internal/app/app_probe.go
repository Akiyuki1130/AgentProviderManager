package app

import (
	"strings"

	"agentprovidermanager/internal/core"
)

// probeResponse 把 core.ProbeReport 压平成前端消费的 map。
// success 以 models 为主：models 通过且（chat 被跳过或 chat 通过）才算连通。
func probeResponse(report core.ProbeReport, kind string) map[string]interface{} {
	ok := report.Models.OK && (report.ChatSkipped || report.Chat.OK)
	var primaryErr, primaryCode string
	if !report.Models.OK {
		primaryErr, primaryCode = report.Models.Error, report.Models.Code
	} else if !report.Chat.OK {
		primaryErr, primaryCode = report.Chat.Error, report.Chat.Code
	}
	return map[string]interface{}{
		"success":      ok,
		"ok":           ok,
		"models_ok":    report.Models.OK,
		"models_count": report.Models.Count,
		"models_ms":    report.Models.MS,
		"chat_ok":      report.Chat.OK,
		"chat_ms":      report.Chat.MS,
		"chat_skipped": report.ChatSkipped,
		"error":        primaryErr,
		"error_code":   primaryCode,
		"kind":         kind,
	}
}

// TestConnectionRaw 直接测试传入的 Base URL / API Key / kind，kind 由前端提供。
func (a *App) TestConnectionRaw(baseURL, apiKey, kind string) map[string]interface{} {
	if strings.TrimSpace(baseURL) == "" {
		return map[string]interface{}{"success": false, "error": "请填写 Base URL", "error_code": "BAD_URL"}
	}
	kindNorm := core.ProbeKind(kind)
	return probeResponse(core.TestConnection(baseURL, apiKey, kindNorm), kindNorm)
}

// TestProviderConnection 解析凭据后运行两段探测，解析逻辑与 RefreshProviderModels 一致。
func (a *App) TestProviderConnection(providerID, baseURLOverride, apiKeyOverride string) map[string]interface{} {
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件", "error_code": "OTHER"}
	}
	// 注册了 backend 的 agent 走统一分发：用 store 的 Get 取 Base URL、凭据与 kind。
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, rerr := backend.Read(target)
		if rerr != nil {
			return map[string]interface{}{"success": false, "error": core.ShortText(rerr.Error(), 300), "error_code": "OTHER"}
		}
		edit, gerr := backend.Store(doc).Get(doc, providerID)
		if gerr != nil {
			return map[string]interface{}{"success": false, "error": core.ShortText(gerr.Error(), 300), "error_code": "OTHER"}
		}
		return probeProvider(edit, baseURLOverride, apiKeyOverride)
	}
	cfg, err := core.LoadConfig(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300), "error_code": "OTHER"}
	}
	edit, err := core.ProviderToEdit(cfg, providerID)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300), "error_code": "OTHER"}
	}
	return probeProvider(edit, baseURLOverride, apiKeyOverride)
}

func probeProvider(edit *core.ProviderEdit, baseURLOverride, apiKeyOverride string) map[string]interface{} {
	baseURL := strings.TrimSpace(baseURLOverride)
	if baseURL == "" {
		baseURL = edit.BaseURL
	}
	apiKey := strings.TrimSpace(apiKeyOverride)
	// 与 app.go 的「重新获取模型」同口径：已存 Key 只在与本次要用的 Base URL 同源时沿用。
	// 否则「把 Base URL 改成任意主机 + 清空 Key 输入框 + 测试连接」会把表单里预填的
	// 本地真实 Key 发给对方——测试连接同样能外带凭据，不能只堵重新获取那条路径。
	if apiKey == "" && acceptsLocalKeyForOverride(edit.BaseURL, baseURLOverride) {
		apiKey = edit.APIKey
	}
	if baseURL == "" {
		return map[string]interface{}{"success": false, "error": "该提供商未配置 Base URL，请先在管理页填写", "error_code": "BAD_URL"}
	}
	kindNorm := core.ProbeKind(edit.Kind)
	return probeResponse(core.TestConnection(baseURL, apiKey, kindNorm), kindNorm)
}

// acceptsLocalKeyForOverride 判定「本次测试是否允许沿用本地已存的 API Key」：
// 没有 Base URL 覆盖时沿用（测的就是这个 provider 自己）；有覆盖时只在与本地
// Base URL 同源（协议 + 主机 + 端口一致）时沿用，来源不明一律不沿用（fail-closed，
// Key 留空，由既有错误路径返回中文提示）。
// 抽成纯函数是为了能直接单测这条闸门，不必真的发起网络请求。
func acceptsLocalKeyForOverride(localBaseURL, override string) bool {
	if override == "" {
		return true
	}
	return core.SameURLOrigin(localBaseURL, override)
}
