package core

import "testing"

// 拨号守卫里「代理放行」这一分支（isConfiguredProxyAddress）只读环境变量、不发任何网络
// 请求，因此可以离线单测。它必须同时守住两个方向：
//  1. 真命中时放行——拨号地址就是环境里配置的代理（含键名/地址大小写差异、省略端口补
//     默认端口、无 scheme 写法）时必须返回 true；否则本机代理环境下「重新获取模型 /
//     测试连接」会被逐 IP 复检整体拦死，且界面无提示；
//  2. fail-closed——环境没有代理、主机/端口对不上、或变量解析不出来时必须返回 false，
//     让 guardedDialContext 继续走逐 IP 复检（DNS rebinding 防线不能被削弱）。
//
// 环境隔离：开发机常常真的配了 HTTP_PROXY，所以每个子用例开头都必须把六个代理键清空，
// 否则用例之间以及和真实环境会互相污染。这里用 t.Setenv(k, "") 而不是 os.Unsetenv：
// 实现用 os.Getenv 读取，并对 strings.TrimSpace(raw) == "" 直接 continue（见 dial.go
// 里遍历 proxyEnvKeys 的循环），所以「未设置」与「空串」在实现里走的是同一条分支，
// 语义等价；而 t.Setenv 会在用例结束时自动还原原值，也不会和 t.Parallel 误用。
func TestIsConfiguredProxyAddress(t *testing.T) {
	proxyKeys := []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"}

	cases := []struct {
		name    string
		env     map[string]string
		address string
		want    bool
	}{
		// 无代理环境：任何地址都不放行。
		{name: "未设置任何代理_回环地址不放行", address: "127.0.0.1:7897", want: false},
		{name: "未设置任何代理_任意私网主机不放行", address: "10.0.0.1:8080", want: false},

		// HTTP_PROXY：显式端口，主机与端口都要对上。
		{name: "HTTP_PROXY_命中自身地址", env: map[string]string{"HTTP_PROXY": "http://127.0.0.1:7897"}, address: "127.0.0.1:7897", want: true},
		{name: "HTTP_PROXY_端口不同不放行", env: map[string]string{"HTTP_PROXY": "http://127.0.0.1:7897"}, address: "127.0.0.1:7898", want: false},
		{name: "HTTP_PROXY_主机不同不放行", env: map[string]string{"HTTP_PROXY": "http://127.0.0.1:7897"}, address: "10.0.0.1:7897", want: false},

		// HTTPS_PROXY：小写键名 + 地址大小写不同仍应命中；显式端口不能被默认 443 覆盖。
		{name: "HTTPS_PROXY_小写键名且地址大小写不同仍命中", env: map[string]string{"https_proxy": "https://Proxy.Example:8443"}, address: "proxy.example:8443", want: true},
		{name: "HTTPS_PROXY_显式端口不被默认443覆盖", env: map[string]string{"HTTPS_PROXY": "https://proxy.example:8443"}, address: "proxy.example:443", want: false},

		// HTTP_PROXY 省略端口：按 http 补 80。
		{name: "HTTP_PROXY_省略端口补80", env: map[string]string{"HTTP_PROXY": "http://proxy.example"}, address: "proxy.example:80", want: true},
		{name: "HTTP_PROXY_省略端口不匹配其它端口", env: map[string]string{"HTTP_PROXY": "http://proxy.example"}, address: "proxy.example:8080", want: false},

		// ALL_PROXY + socks5：net/http 对省略端口的 socks5 按 1080 建连，这里必须同口径。
		{name: "ALL_PROXY_socks5省略端口补1080", env: map[string]string{"ALL_PROXY": "socks5://127.0.0.1"}, address: "127.0.0.1:1080", want: true},
		{name: "ALL_PROXY_socks5大写scheme也补1080", env: map[string]string{"ALL_PROXY": "SOCKS5://127.0.0.1"}, address: "127.0.0.1:1080", want: true},
		{name: "ALL_PROXY_socks5显式端口命中", env: map[string]string{"all_proxy": "socks5://127.0.0.1:1080"}, address: "127.0.0.1:1080", want: true},
		{name: "ALL_PROXY_socks5不匹配其它端口", env: map[string]string{"ALL_PROXY": "socks5://127.0.0.1"}, address: "127.0.0.1:7897", want: false},

		// 省略 scheme 的常见写法：按 http:// 处理。
		{name: "HTTP_PROXY_无scheme写法按http处理", env: map[string]string{"HTTP_PROXY": "127.0.0.1:7897"}, address: "127.0.0.1:7897", want: true},
		{name: "HTTP_PROXY_无scheme写法端口不同不放行", env: map[string]string{"HTTP_PROXY": "127.0.0.1:7897"}, address: "127.0.0.1:7898", want: false},

		// 解析不出来的值：不 panic、不放行（fail-closed）。
		{name: "无法解析的值不panic且不放行", env: map[string]string{"HTTP_PROXY": "://nope"}, address: "127.0.0.1:7897", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 先把六个代理键一律置空（等价于未设置，见文件顶部的说明），再写入本用例需要的值，
			// 保证用例之间与真实环境互不污染。
			for _, key := range proxyKeys {
				t.Setenv(key, "")
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			if got := isConfiguredProxyAddress(tc.address); got != tc.want {
				t.Fatalf("isConfiguredProxyAddress(%q) = %v, want %v（env=%v）", tc.address, got, tc.want, tc.env)
			}
		})
	}
}
