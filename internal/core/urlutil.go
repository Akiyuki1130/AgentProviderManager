package core

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

func isPrivateOrReservedHost(host string) bool {
	low := strings.ToLower(host)
	if low == "localhost" || low == "::1" || strings.HasSuffix(low, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}
	return false
}

func ValidateBaseURL(raw string) (bool, string) {
	if strings.TrimSpace(raw) == "" {
		return false, "请填写 Base URL"
	}
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false, "Base URL 的端口格式无效"
	}
	// Validate port by checking hostname parsing
	if parsed.Host == "" && trimmed != "" {
		return false, "Base URL 缺少有效主机名"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false, "Base URL 仅支持 http/https 协议"
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false, "Base URL 缺少有效主机名"
	}
	if regexp.MustCompile(`[\s\x00-\x1f\x7f]`).MatchString(host) {
		return false, "Base URL 的主机名包含空白或非法字符"
	}
	if parsed.User != nil {
		return false, "Base URL 不允许包含用户名或密码"
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return false, "Base URL 不允许包含查询参数或片段"
	}
	isLocal := host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".localhost")
	if parsed.Scheme != "https" && !isLocal {
		return false, "为保护 API Key，非本机地址必须使用 https://"
	}
	return true, ""
}

func ValidateFetchURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("仅允许 http/https 协议")
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("缺少有效主机名")
	}
	if isPrivateOrReservedHost(host) {
		return fmt.Errorf("已拦截私有/保留地址请求")
	}
	return nil
}

func NormalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(strings.ToLower(path), "/models") {
		path = strings.TrimRight(path[:len(path)-len("/models")], "/")
	}
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	return parsed.String()
}

func ModelsURL(baseURL string) string {
	base := NormalizeBaseURL(baseURL)
	parsed, _ := url.Parse(base)
	path := strings.TrimRight(parsed.Path, "/")
	if !regexp.MustCompile(`/v\d+[a-zA-Z0-9._-]*$`).MatchString(path) {
		path += "/v1"
	}
	path += "/models"
	parsed.Path = path
	return parsed.String()
}

func ShortText(text string, limit int) string {
	s := strings.TrimSpace(text)
	if len([]rune(s)) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "…"
}
