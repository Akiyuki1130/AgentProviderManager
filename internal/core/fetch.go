package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

func RedactSensitive(text string, secrets []string) string {
	value := text
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	value = regexp.MustCompile(`(?i)(bearer\s+)[^\s,;\"']+`).ReplaceAllString(value, "${1}[REDACTED]")
	value = regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)\s*[:=]\s*)[^\s,;\"']+`).ReplaceAllString(value, "${1}[REDACTED]")
	return value
}

func serverErrorMessage(raw []byte) string {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	errVal, ok := data["error"]
	if !ok {
		return ""
	}
	switch v := errVal.(type) {
	case map[string]interface{}:
		if msg, ok := v["message"].(string); ok {
			if len(msg) > 200 {
				return msg[:200]
			}
			return msg
		}
	case string:
		if len(v) > 200 {
			return v[:200]
		}
		return v
	}
	return ""
}

// FetchOptions 是模型拉取的可选认证参数，专为 omp provider 的自由 headers / authHeader 引入。
//
//   - Headers：omp provider 的 headers 原样透传；值可以写成环境变量名或 !command 引用。
//     与原生认证头同名（大小写不敏感）时以自定义头为准（G9）。
//   - Kind：三种主流协议之一；anthropic 用 x-api-key + anthropic-version，其余用 Bearer。
//   - AuthHeader：对应 omp 的 authHeader。只有显式为 true 且有 Key 时才补
//     Authorization: Bearer；false / nil 都不关闭协议原生认证（G5）。
type FetchOptions struct {
	Headers    map[string]string
	Kind       string
	AuthHeader *bool
}

// FetchModelsRaw fetches model list from baseURL.
// 保留旧入口：app（zcode/DSH 兜底路径）与既有测试继续调用，行为等价于空 FetchOptions。
func FetchModelsRaw(baseURL, apiKey string, timeout time.Duration) ([]struct {
	ID   string
	Meta map[string]interface{}
}, error) {
	return FetchModelsWithOptions(baseURL, apiKey, FetchOptions{}, timeout)
}

// FetchModelsWithOptions 是带 omp 认证口径的模型拉取入口：SSRF 校验、逐跳重定向复检、
// 跨主机凭据剥离与 RedactSensitive 全部走同一份实现（app 层不要另写网络代码）。
func FetchModelsWithOptions(baseURL, apiKey string, opt FetchOptions, timeout time.Duration) ([]struct {
	ID   string
	Meta map[string]interface{}
}, error) {
	if ok, msg := ValidateBaseURL(baseURL); !ok {
		return nil, NewModelFetchError(msg, "BAD_URL")
	}
	fetchURL := ModelsURL(baseURL)
	if err := ValidateFetchURL(fetchURL); err != nil {
		return nil, NewModelFetchError(fmt.Sprintf("请求被拦截：%v", err), "BAD_URL")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	// omp 的 apiKey 可能是 !command 或环境变量名：Go 端绝不执行命令，环境变量名按 omp
	// 规则精确解析（未命中则按字面量使用，绝不展开 $VAR）。
	key, err := resolveAPIKeyValue(apiKey)
	if err != nil {
		return nil, err
	}

	client := newGuardedClient(timeout, func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	})

	headers, secrets := buildOutboundHeaders(key, opt)
	secrets = append(secrets, strings.TrimSpace(apiKey), key)

	originalHost := ""
	if u, err := url.Parse(fetchURL); err == nil {
		originalHost = u.Host
	}

	currentURL := fetchURL
	maxBytes := 10 * 1024 * 1024
	deadline := time.Now().Add(timeout)

	for i := 0; i < 6; i++ {
		if time.Now().After(deadline) {
			return nil, NewModelFetchError("请求超时，请稍后重试", "TIMEOUT")
		}
		if err := ValidateFetchURL(currentURL); err != nil {
			return nil, NewModelFetchError(fmt.Sprintf("请求被重定向到不安全地址：%v", err), "BAD_URL")
		}
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return nil, NewModelFetchError(fmt.Sprintf("请求构造失败：%v", err), "BAD_URL")
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, NewModelFetchError(RedactSensitive(err.Error(), secrets), "CONN")
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			resp.Body.Close()
			if location == "" {
				return nil, NewModelFetchError("服务器返回了缺少 Location 的重定向", "BAD_URL")
			}
			base, _ := url.Parse(currentURL)
			nextURL, err := base.Parse(location)
			if err != nil {
				return nil, NewModelFetchError("重定向地址解析失败", "BAD_URL")
			}
			nextStr := nextURL.String()
			if ok, msg := ValidateBaseURL(nextStr); !ok {
				return nil, NewModelFetchError(fmt.Sprintf("请求被重定向到不安全地址：%s", msg), "BAD_URL")
			}
			if err := ValidateFetchURL(nextStr); err != nil {
				return nil, NewModelFetchError(fmt.Sprintf("请求被重定向到不安全地址：%v", err), "BAD_URL")
			}
			applyRedirectHeaders(headers, nextURL.Host, originalHost, key, opt)
			currentURL = nextStr
			continue
		}

		// Read body with size limit
		limited := io.LimitReader(resp.Body, int64(maxBytes+1))
		raw, err := io.ReadAll(limited)
		resp.Body.Close()
		if err != nil {
			return nil, NewModelFetchError(RedactSensitive(err.Error(), secrets), "BAD_RESPONSE")
		}
		if len(raw) > maxBytes {
			return nil, NewModelFetchError(fmt.Sprintf("响应内容过大（>%dMB），已中止读取", maxBytes/(1024*1024)), "BAD_RESPONSE")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg := serverErrorMessage(raw)
			if msg == "" {
				text := strings.TrimSpace(string(raw))
				if len(text) > 200 {
					text = text[:200]
				}
				if text != "" {
					msg = text
				} else {
					msg = resp.Status
				}
			}
			msg = RedactSensitive(msg, secrets)
			return nil, NewModelFetchError(fmt.Sprintf("服务器返回错误（HTTP %d）：%s", resp.StatusCode, msg), fmt.Sprintf("HTTP_%d", resp.StatusCode))
		}
		// Parse JSON
		var data interface{}
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, NewModelFetchError("响应不是有效的 JSON（服务器可能返回了错误页面或登录提示）", "BAD_RESPONSE")
		}
		var items []interface{}
		if m, ok := data.(map[string]interface{}); ok {
			if d, ok := m["data"]; ok {
				if arr, ok := d.([]interface{}); ok {
					items = arr
				}
			}
		} else if arr, ok := data.([]interface{}); ok {
			items = arr
		}
		result := []struct {
			ID   string
			Meta map[string]interface{}
		}{}
		for _, item := range items {
			switch v := item.(type) {
			case map[string]interface{}:
				mid, _ := v["id"].(string)
				if mid == "" {
					mid, _ = v["name"].(string)
				}
				mid = strings.TrimSpace(mid)
				if mid == "" {
					continue
				}
				if len(mid) > MaxModelIDLength {
					return nil, NewModelFetchError(fmt.Sprintf("模型 ID 过长（上限 %d 个字符）", MaxModelIDLength), "BAD_RESPONSE")
				}
				result = append(result, struct {
					ID   string
					Meta map[string]interface{}
				}{ID: mid, Meta: v})
			case string:
				mid := strings.TrimSpace(v)
				if mid == "" {
					continue
				}
				result = append(result, struct {
					ID   string
					Meta map[string]interface{}
				}{ID: mid, Meta: map[string]interface{}{}})
			}
			if len(result) > MaxModels {
				return nil, NewModelFetchError(fmt.Sprintf("模型数量过多（上限 %d 个）", MaxModels), "BAD_RESPONSE")
			}
		}
		// deduplicate and sort
		seen := map[string]bool{}
		var unique []struct {
			ID   string
			Meta map[string]interface{}
		}
		for _, r := range result {
			if !seen[r.ID] {
				seen[r.ID] = true
				unique = append(unique, r)
			}
		}
		sort.Slice(unique, func(i, j int) bool {
			return strings.ToLower(unique[i].ID) < strings.ToLower(unique[j].ID)
		})
		return unique, nil
	}
	return nil, NewModelFetchError("服务器重定向次数过多", "BAD_URL")
}

// applyRedirectAuth 只在重定向回到原始主机时保留 Bearer 凭据：
// 主机一变就删除，避免把 API Key 转发给第三方；空 Key 时不会写出 "Bearer "，
// 与首次请求（不带 Authorization 头）保持一致。
func applyRedirectAuth(headers map[string]string, nextHost, originalHost, apiKey string) {
	key := strings.TrimSpace(apiKey)
	if key != "" && nextHost == originalHost {
		headers["Authorization"] = "Bearer " + key
		return
	}
	delete(headers, "Authorization")
}

// resolveAPIKeyValue 按 omp 的 apiKey 约定解析：以 ! 开头是 shell 命令，Go 端永不执行，
// 直接拒绝（KEY_COMMAND）；否则整串当作环境变量名精确查找，命中且非空就用它的值；
// 都未命中则按字面量密钥使用（omp 不展开 $VAR，G4）。
func resolveAPIKeyValue(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "!") {
		return "", NewModelFetchError("该提供商的 API Key 是 shell 命令（以 ! 开头），本程序不会执行命令；请在 omp 侧改为字面量或环境变量名后重试", "KEY_COMMAND")
	}
	if value, ok := os.LookupEnv(trimmed); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), nil
	}
	return trimmed, nil
}

// resolveHeaderValue 与 apiKey 同规则；!command 返回 ok=false（该头不发，也绝不执行命令）。
func resolveHeaderValue(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, "!") {
		return "", false
	}
	if value, ok := os.LookupEnv(trimmed); ok && strings.TrimSpace(value) != "" {
		return value, true
	}
	return raw, true
}

// buildOutboundHeaders 先放自定义头（omp provider.headers），再补协议原生认证头；
// 同名时自定义头优先（G9）。返回的 secrets 用于所有对外错误的 RedactSensitive。
func buildOutboundHeaders(apiKey string, opt FetchOptions) (map[string]string, []string) {
	headers := map[string]string{}
	secrets := []string{}
	key := strings.TrimSpace(apiKey)
	if key != "" {
		secrets = append(secrets, key)
	}
	for name, rawValue := range opt.Headers {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if canonical == "" || strings.HasPrefix(canonical, "!") {
			continue
		}
		value, ok := resolveHeaderValue(rawValue)
		if !ok {
			continue
		}
		headers[canonical] = value
		secrets = append(secrets, value)
	}
	if normalizeProbeKind(opt.Kind) == "anthropic" {
		if key != "" && headers["X-Api-Key"] == "" {
			headers["X-Api-Key"] = key
		}
		if headers["Anthropic-Version"] == "" {
			headers["Anthropic-Version"] = "2023-06-01"
		}
	} else if key != "" && headers["Authorization"] == "" {
		headers["Authorization"] = "Bearer " + key
	}
	// authHeader=true 是 config 层补 Authorization: Bearer 的开关，与原生认证并行不冲突。
	if opt.AuthHeader != nil && *opt.AuthHeader && key != "" && headers["Authorization"] == "" {
		headers["Authorization"] = "Bearer " + key
	}
	return headers, secrets
}

// stripSensitiveHeaders 在跨主机重定向时 fail-closed 地删掉一切可能是凭据的请求头：
// 名字命中 auth/key/token/secret/credential/cookie，或值里含本次 Key 的头一律不发。
func stripSensitiveHeaders(headers map[string]string, apiKey string) {
	key := strings.TrimSpace(apiKey)
	for name, value := range headers {
		lower := strings.ToLower(name)
		sensitiveName := lower == "cookie" || strings.Contains(lower, "auth") ||
			strings.Contains(lower, "key") || strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "credential")
		if sensitiveName || (key != "" && strings.Contains(value, key)) {
			delete(headers, name)
		}
	}
}

// applyRedirectHeaders 是重定向时的请求头策略：同主机续用原生认证头（保持既有
// applyRedirectAuth 的语义与测试口径），跨主机一律剥离凭据类头（含自定义头）。
func applyRedirectHeaders(headers map[string]string, nextHost, originalHost, apiKey string, opt FetchOptions) {
	if nextHost != originalHost {
		stripSensitiveHeaders(headers, apiKey)
		return
	}
	key := strings.TrimSpace(apiKey)
	if headers["Authorization"] == "" {
		applyRedirectAuth(headers, nextHost, originalHost, apiKey)
	}
	if normalizeProbeKind(opt.Kind) == "anthropic" {
		if key != "" && headers["X-Api-Key"] == "" {
			headers["X-Api-Key"] = key
		}
		if headers["Anthropic-Version"] == "" {
			headers["Anthropic-Version"] = "2023-06-01"
		}
	}
	if opt.AuthHeader != nil && *opt.AuthHeader && key != "" && headers["Authorization"] == "" {
		headers["Authorization"] = "Bearer " + key
	}
}
