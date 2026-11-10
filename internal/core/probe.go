package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ProbeStep 是单段探测的结果：耗时、模型数、失败信息与错误码。
type ProbeStep struct {
	OK    bool
	MS    int64
	Count int
	Error string
	Code  string
}

// ProbeReport 两段探测报告：/models 可达性 + chat 端点鉴权实测。
type ProbeReport struct {
	Models ProbeStep
	Chat   ProbeStep
	// ChatSkipped 为 true 表示 chat 实测未执行：/models 阶段失败，或模型列表为空。
	ChatSkipped bool
}

// ProbeOptions 与 FetchOptions 同形，供 omp provider 的 headers / authHeader / kind 探测。
//
//   - Headers：provider 自有请求头，透传给 /models 与 chat 两段；同名时优先于原生认证头。
//   - Kind：三种主流协议之一（anthropic 走 x-api-key，responses/openai-compatible 走 Bearer）。
//   - AuthHeader：omp 的 authHeader，只有显式 true 才补 Authorization: Bearer（G5）。
type ProbeOptions struct {
	Headers    map[string]string
	Kind       string
	AuthHeader *bool
}

// ProbeKind 归一化 kind；未识别或为空时回落到 openai-compatible。
func ProbeKind(kind string) string {
	if norm, err := ValidateKind(kind); err == nil {
		return norm
	}
	return "openai-compatible"
}

// normalizeProbeKind 把 APM / omp 两套写法归一成三种 UI kind 之一：
// 空值保持为空（由调用方决定回落），config 层写法（openai）按 ConfigKindToUI 映射，
// 其余未知协议原样保留（探测与拉取按非 anthropic 走 Bearer，与旧行为一致）。
func normalizeProbeKind(kind string) string {
	trimmed := strings.TrimSpace(kind)
	if trimmed == "" {
		return ""
	}
	if mapped, ok := ConfigKindToUI[trimmed]; ok {
		return mapped
	}
	return trimmed
}

// TestConnection 两段探测：/models 可达性 + 1-token chat 端点鉴权实测。kind 未识别时按 openai-compatible。
// 保留旧入口：app 与既有测试继续调用，等价于空 ProbeOptions。
func TestConnection(baseURL, apiKey, kind string) ProbeReport {
	return ProbeProviderRequest(baseURL, apiKey, ProbeOptions{Kind: kind}, 15*time.Second)
}

// ProbeProviderRequest 是带 omp 认证口径的探测入口：两段探测都复用 fetch.go 的同一份
// SSRF / 重定向 / RedactSensitive 逻辑，app 层不要另写网络代码。
func ProbeProviderRequest(baseURL, apiKey string, opt ProbeOptions, timeout time.Duration) ProbeReport {
	report := ProbeReport{ChatSkipped: true}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	secrets := probeSecrets(apiKey, opt.Headers)
	start := time.Now()
	models, err := FetchModelsWithOptions(baseURL, apiKey, FetchOptions{
		Headers:    opt.Headers,
		Kind:       opt.Kind,
		AuthHeader: opt.AuthHeader,
	}, timeout)
	report.Models.MS = time.Since(start).Milliseconds()
	if err != nil {
		code, msg := "OTHER", err.Error()
		if mfe, ok := err.(*ModelFetchError); ok {
			code, msg = mfe.ErrorCode, mfe.Msg
		}
		report.Models.Error = ShortText(RedactSensitive(msg, secrets), 200)
		report.Models.Code = code
		return report
	}
	report.Models.OK = true
	report.Models.Count = len(models)
	if len(models) == 0 {
		// 没有可用 model id 就无法做 chat 实测，保持跳过而不是拿空 model 发请求误报失败。
		return report
	}
	report.ChatSkipped = false

	model := models[0].ID
	root := strings.TrimSuffix(ModelsURL(baseURL), "/models")
	chatStart := time.Now()
	report.Chat = probeChat(root, model, apiKey, opt, timeout)
	report.Chat.MS = time.Since(chatStart).Milliseconds()
	return report
}

// probeChat 向 kind 对应的 chat 端点发一条 1-token 消息实测鉴权。
// 与 fetch.go 姿态一致但不跟随重定向：任何 3xx 都按失败处理，避免跨主机转发凭据。
func probeChat(root, model, apiKey string, opt ProbeOptions, timeout time.Duration) ProbeStep {
	secrets := probeSecrets(apiKey, opt.Headers)
	fail := func(code, msg string) ProbeStep {
		return ProbeStep{Code: code, Error: ShortText(RedactSensitive(msg, secrets), 200)}
	}
	// apiKey 可能是 !command / 环境变量名：与 /models 段同口径解析，命令永不执行。
	key, err := resolveAPIKeyValue(apiKey)
	if err != nil {
		code, msg := "OTHER", err.Error()
		if mfe, ok := err.(*ModelFetchError); ok {
			code, msg = mfe.ErrorCode, mfe.Msg
		}
		return fail(code, msg)
	}
	headers, headerSecrets := buildOutboundHeaders(key, FetchOptions{
		Headers:    opt.Headers,
		Kind:       opt.Kind,
		AuthHeader: opt.AuthHeader,
	})
	secrets = append(secrets, headerSecrets...)
	headers["Content-Type"] = "application/json"

	kind := normalizeProbeKind(opt.Kind)
	var endpoint string
	var payload map[string]interface{}
	switch kind {
	case "anthropic":
		endpoint = root + "/messages"
		payload = map[string]interface{}{
			"model":      model,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		}
	case "responses":
		endpoint = root + "/responses"
		payload = map[string]interface{}{
			"model":             model,
			"input":             "ping",
			"max_output_tokens": 1,
		}
	default:
		endpoint = root + "/chat/completions"
		payload = map[string]interface{}{
			"model":      model,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		}
	}
	if err := ValidateFetchURL(endpoint); err != nil {
		return fail("BAD_URL", fmt.Sprintf("请求被拦截：%v", err))
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fail("OTHER", "请求构造失败")
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fail("BAD_URL", err.Error())
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := newGuardedClient(timeout, func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	})
	resp, err := client.Do(req)
	if err != nil {
		code := "CONN"
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			code = "TIMEOUT"
		}
		return fail(code, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fail("HTTP_3xx", "chat 端点返回重定向，无法跟随")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fail("BAD_RESPONSE", err.Error())
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ProbeStep{OK: true}
	}
	msg := serverErrorMessage(raw)
	if msg == "" {
		msg = resp.Status
	}
	return fail(fmt.Sprintf("HTTP_%d", resp.StatusCode), msg)
}

// probeSecrets 收集需要从对外错误里抹掉的凭据：原始 apiKey 与 provider 自有头的值。
func probeSecrets(apiKey string, headers map[string]string) []string {
	secrets := []string{strings.TrimSpace(apiKey)}
	for _, value := range headers {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			secrets = append(secrets, trimmed)
		}
	}
	return secrets
}
