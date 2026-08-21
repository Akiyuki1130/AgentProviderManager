package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// FetchModelsRaw fetches model list from baseURL.
func FetchModelsRaw(baseURL, apiKey string, timeout time.Duration) ([]struct {
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

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	headers := map[string]string{}
	if strings.TrimSpace(apiKey) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(apiKey)
	}

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
			return nil, NewModelFetchError(RedactSensitive(err.Error(), []string{apiKey}), "CONN")
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
			if nextURL.Host != originalHost && nextURL.Host != mustParse(currentURL).Host {
				delete(headers, "Authorization")
			} else if nextURL.Host == originalHost && strings.TrimSpace(apiKey) != "" {
				headers["Authorization"] = "Bearer " + strings.TrimSpace(apiKey)
			}
			// Same-host redirect keeps auth
			if nextURL.Host == mustParse(currentURL).Host {
				// keep headers
			} else if nextURL.Host == originalHost {
				headers["Authorization"] = "Bearer " + strings.TrimSpace(apiKey)
			}
			currentURL = nextStr
			continue
		}

		// Read body with size limit
		limited := io.LimitReader(resp.Body, int64(maxBytes+1))
		raw, err := io.ReadAll(limited)
		resp.Body.Close()
		if err != nil {
			return nil, NewModelFetchError(RedactSensitive(err.Error(), []string{apiKey}), "BAD_RESPONSE")
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
			msg = RedactSensitive(msg, []string{apiKey})
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

func mustParse(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}
