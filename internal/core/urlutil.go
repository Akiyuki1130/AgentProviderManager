package core

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// httpPolicy guards the "allow http://" option. Default is off: only https is
// accepted.
var (
	httpPolicyMu sync.RWMutex
	httpPolicyOn bool
)

// privatePolicy guards the "allow local/private addresses" option. Default is
// off: localhost / loopback / private / reserved hosts are rejected.
var (
	privatePolicyMu sync.RWMutex
	privatePolicyOn bool
)

// SetHTTPAllowed controls whether plain http:// URLs are accepted.
func SetHTTPAllowed(v bool) {
	httpPolicyMu.Lock()
	httpPolicyOn = v
	httpPolicyMu.Unlock()
}

// HTTPAllowed reports whether plain http:// URLs are accepted.
func HTTPAllowed() bool {
	httpPolicyMu.RLock()
	defer httpPolicyMu.RUnlock()
	return httpPolicyOn
}

// SetPrivateHostsAllowed controls whether localhost/loopback/private/reserved
// hosts are accepted.
func SetPrivateHostsAllowed(v bool) {
	privatePolicyMu.Lock()
	privatePolicyOn = v
	privatePolicyMu.Unlock()
}

// PrivateHostsAllowed reports whether private/reserved hosts are accepted.
func PrivateHostsAllowed() bool {
	privatePolicyMu.RLock()
	defer privatePolicyMu.RUnlock()
	return privatePolicyOn
}

// reservedNets 补齐 SECURITY.md 承诺拒绝、但 net 标准库的 IsPrivate / IsLoopback
// 不覆盖的保留地址段（运营商级 NAT、测试网、基准测试网、6to4、文档地址等）。
// 逐段比对而不是枚举前缀，避免写出新的漏判分支。
var reservedNets = func() []*net.IPNet {
	raw := []string{
		"100.64.0.0/10",   // 运营商级 NAT（RFC 6598）
		"192.0.0.0/24",    // IETF 协议专用
		"192.0.2.0/24",    // TEST-NET-1
		"192.88.99.0/24",  // 6to4 中继（已废弃）
		"198.18.0.0/15",   // 基准测试
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // 保留（含 255.255.255.255 广播）
		"100::/64",        // IPv6 丢弃前缀
		"2001:db8::/32",   // IPv6 文档地址
		"2002::/16",       // 6to4（内嵌 IPv4，可指向内网）
		"64:ff9b::/96",    // NAT64 知名前缀
	}
	out := make([]*net.IPNet, 0, len(raw))
	for _, cidr := range raw {
		if _, block, err := net.ParseCIDR(cidr); err == nil {
			out = append(out, block)
		}
	}
	return out
}()

func isPrivateOrReservedHost(host string) bool {
	low := strings.ToLower(strings.TrimSpace(host))
	// IPv6 作用域地址（如 fe80::1%eth0）带 zone 后缀，net.ParseIP 会拒绝整个
	// 字符串并返回 nil，从而绕过下面的私有地址判断；先去掉 zone 再解析。
	if i := strings.IndexByte(low, '%'); i >= 0 {
		low = low[:i]
	}
	// 绝对形式（"localhost." / "127.0.0.1."）同样会让 net.ParseIP 返回 nil 并
	// 与下面的后缀判断失配，先去掉根点再判。
	low = strings.TrimSuffix(low, ".")
	if low == "localhost" || low == "::1" || strings.HasSuffix(low, ".localhost") || strings.HasSuffix(low, ".local") {
		return true
	}
	ip := net.ParseIP(low)
	if ip == nil {
		return false
	}
	return isPrivateOrReservedIP(ip)
}

// isPrivateOrReservedIP 判断解析后的地址是否属于必须拒绝的范围。
// 拨号守卫（dial.go）与 URL 校验共用它，保证「URL 里写的」和「实际连的」同一口径。
func isPrivateOrReservedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, block := range reservedNets {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// SameURLOrigin 比较两个 URL 的「协议 + 主机 + 端口」是否一致，用于「Base URL 换到
// 别的主机后不得沿用本地已存凭据」这类判断。任一侧无法解析、缺少主机名或协议不同
// 都按「不同源」处理（fail-closed：宁可要求重新填写 Key，也不把凭据发给未知主机）。
func SameURLOrigin(a, b string) bool {
	ua, errA := url.Parse(strings.TrimSpace(a))
	ub, errB := url.Parse(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return false
	}
	if !strings.EqualFold(ua.Scheme, ub.Scheme) {
		return false
	}
	hostA := strings.TrimSuffix(strings.ToLower(ua.Hostname()), ".")
	hostB := strings.TrimSuffix(strings.ToLower(ub.Hostname()), ".")
	if hostA == "" || hostB == "" {
		return false
	}
	return hostA == hostB && effectivePort(ua) == effectivePort(ub)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
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
	if parsed.Scheme == "http" && !HTTPAllowed() {
		return false, "Base URL 仅支持 https 协议（如需使用 http 请在选项中开启 HTTP 支持）"
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
	return true, ""
}

func ValidateFetchURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("仅允许 http/https 协议")
	}
	if parsed.Scheme == "http" && !HTTPAllowed() {
		return fmt.Errorf("仅允许 https 协议（如需使用 http 请在选项中开启 HTTP 支持）")
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("缺少有效主机名")
	}
	if !PrivateHostsAllowed() && isPrivateOrReservedHost(host) {
		return fmt.Errorf("已拦截私有/保留地址请求（如需访问本地或内网地址，请在选项中开启\"允许本地与内网地址\"）")
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
