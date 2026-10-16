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

// ProbeKind 归一化 kind；未识别或为空时回落到 openai-compatible。
func ProbeKind(kind string) string {
	if norm, err := ValidateKind(kind); err == nil {
		return norm
	}
	return "openai-compatible"
}

// TestConnection 两段探测：/models 可达性 + 1-token chat 端点鉴权实测。kind 未识别时按 openai-compatible。
func TestConnection(baseURL, apiKey, kind string) ProbeReport {
	report := ProbeReport{ChatSkipped: true}
	start := time.Now()
	models, err := FetchModelsRaw(baseURL, apiKey, 15*time.Second)
	report.Models.MS = time.Since(start).Milliseconds()
	if err != nil {
		code, msg := "OTHER", err.Error()
		if mfe, ok := err.(*ModelFetchError); ok {
			code, msg = mfe.ErrorCode, mfe.Msg
		}
		report.Models.Error = ShortText(RedactSensitive(msg, []string{apiKey}), 200)
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
	report.Chat = probeChat(root, model, apiKey, ProbeKind(kind))
	report.Chat.MS = time.Since(chatStart).Milliseconds()
	return report
}

// probeChat 向 kind 对应的 chat 端点发一条 1-token 消息实测鉴权。
// 与 fetch.go 姿态一致但不跟随重定向：任何 3xx 都按失败处理，避免跨主机转发凭据。
func probeChat(root, model, apiKey, kind string) ProbeStep {
	fail := func(code, msg string) ProbeStep {
		return ProbeStep{Code: code, Error: ShortText(RedactSensitive(msg, []string{apiKey}), 200)}
	}
	key := strings.TrimSpace(apiKey)
	var endpoint string
	var payload map[string]interface{}
	headers := map[string]string{"Content-Type": "application/json"}
	switch kind {
	case "anthropic":
		endpoint = root + "/messages"
		payload = map[string]interface{}{
			"model":      model,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		}
		if key != "" {
			headers["x-api-key"] = key
		}
		headers["anthropic-version"] = "2023-06-01"
	case "responses":
		endpoint = root + "/responses"
		payload = map[string]interface{}{
			"model":             model,
			"input":             "ping",
			"max_output_tokens": 1,
		}
		if key != "" {
			headers["Authorization"] = "Bearer " + key
		}
	default:
		endpoint = root + "/chat/completions"
		payload = map[string]interface{}{
			"model":      model,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		}
		if key != "" {
			headers["Authorization"] = "Bearer " + key
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
	client := newGuardedClient(15*time.Second, func(*http.Request, []*http.Request) error {
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
