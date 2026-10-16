package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// 出站请求的统一拨号守卫。
//
// urlutil 的地址校验只看得到 URL 里的主机名：域名可以先通过校验、再解析到内网
// （DNS rebinding），也能用别的主机名指向私有地址。因此在真正建连之前必须按
// 解析出来的 IP 再复检一次。语义与 internal/update 的 safeDialContext 一致——
// core 不能反向依赖 update 包，这里保留同口径的独立实现。
//
// 用户显式打开「允许本地与内网地址」时（PrivateHostsAllowed）放行，否则会连本机
// 测试服务器都请求不了。
func guardedDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid network address: %w", err)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve host %s: %w", host, err)
	}
	// 走代理时 Transport 传下来的是**代理地址**（connectMethod.addr() 返回代理），
	// 此时拨号期根本看不到真实目标，对代理做主机的私网判定只会把用户自己配置的代理
	// （如 127.0.0.1:7897）拦死；目标主机的私网判定改由 URL 层逐跳校验负责
	// （ValidateFetchURL / ValidateBaseURL）。因此只有拨号地址「就是环境里配置的代理」
	// 时才跳过下面的 IP 复检：其余情况（环境没有代理、或 NO_PROXY 命中而直连真实目标）
	// 仍逐 IP 复检，DNS rebinding 防线未被削弱。
	allowPrivate := PrivateHostsAllowed() || isConfiguredProxyAddress(net.JoinHostPort(host, port))
	var lastErr error
	for _, ip := range ips {
		if !allowPrivate && isPrivateOrReservedIP(ip) {
			return nil, fmt.Errorf("host %s resolved to a private or reserved address", host)
		}
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("host %s has no resolved addresses", host)
}

// newGuardedClient 返回带「解析后 IP 复检」的 http.Client。重定向策略由调用方给出：
// fetch 逐跳自行处理（保留跨主机剥离凭据的语义），probe 直接拒绝任何 3xx。
//
// 注意 Clone 会一并保留 DefaultTransport 的 Proxy: http.ProxyFromEnvironment：走代理时
// 拨号地址是代理而非真实目标，这条路径必须由 guardedDialContext 的代理放行兜住，
// 否则「重新获取模型 / 测试连接」会在本机代理环境下整体失败（界面无提示）。
// internal/update/update.go 的 safeClient 有同样的 Clone + Proxy 写法，属既有存量问题，
// 本轮不动（已记入 TODO.md）。
func newGuardedClient(timeout time.Duration, redirect func(*http.Request, []*http.Request) error) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = guardedDialContext
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: redirect}
}

// proxyEnvKeys 是可能配置出站代理的环境变量（大小写两套，口径同 net/http 的
// ProxyFromEnvironment，另加 ALL_PROXY）。Go 只认环境变量，不读 Windows 系统代理设置。
var proxyEnvKeys = []string{
	"HTTP_PROXY", "http_proxy",
	"HTTPS_PROXY", "https_proxy",
	"ALL_PROXY", "all_proxy",
}

// isConfiguredProxyAddress 判断 Transport 传下来的拨号地址是否「就是环境里配置的代理」。
// 命中才放行——代理是用户自己配的、就是他明确要连的目标，不需要再问它是不是私网。
// 环境变量缺失或解析不出主机名时一律返回 false，回退到原来的逐 IP 复检（fail-closed）。
func isConfiguredProxyAddress(address string) bool {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return false
	}
	want := strings.ToLower(net.JoinHostPort(host, port))
	for _, key := range proxyEnvKeys {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		u, err := parseProxyURL(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		got := net.JoinHostPort(u.Hostname(), proxyURLPort(u))
		if strings.EqualFold(got, want) {
			return true
		}
	}
	return false
}

// parseProxyURL 解析代理环境变量。net/url 对「host:port」这种省略 scheme 的写法会直接
// 报错，而 net/http 自己是靠补一个 http:// 前缀接住的；保持同一口径，免得环境里常见的
// 无 scheme 写法被误判成解析失败、又退回复检而拦掉用户自己的代理。
func parseProxyURL(raw string) (*url.URL, error) {
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		return u, nil
	}
	return url.Parse("http://" + raw)
}

// proxyURLPort 按 scheme 补代理的默认端口（与 net/http 的 canonicalAddr 同口径）。
// socks5 必须补 1080：net/http 自己对 socks5 代理就是按 1080 建连的，而本函数产出的
// 端口要拿去和 Transport 传下来的拨号地址比对——少了这一档，ALL_PROXY=socks5://host
// （省略端口）会被判成「不是代理」，用户自己配的代理反而被逐 IP 复检拦死。
// 其余 scheme 无法确定端口，返回空串使其不匹配——匹配不到就退回逐 IP 复检，宁可多拦也不误放。
func proxyURLPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	case "socks5":
		return "1080"
	}
	return ""
}
