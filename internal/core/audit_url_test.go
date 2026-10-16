package core

import "testing"

// 带 zone 的 IPv6 link-local 地址必须按私有地址拦截，否则可以绕过 SSRF 校验。
func TestValidateFetchURLBlocksZonedIPv6(t *testing.T) {
	prevHTTP, prevPrivate := HTTPAllowed(), PrivateHostsAllowed()
	SetHTTPAllowed(false)
	SetPrivateHostsAllowed(false)
	t.Cleanup(func() {
		SetHTTPAllowed(prevHTTP)
		SetPrivateHostsAllowed(prevPrivate)
	})
	for _, u := range []string{
		"https://[fe80::1%25eth0]/v1/models",
		"https://[::1%25eth0]/v1/models",
		"https://[fd00::1%25eth0]/v1/models",
	} {
		if err := ValidateFetchURL(u); err == nil {
			t.Errorf("%s 应按私有/保留地址拦截", u)
		}
	}
	if err := ValidateFetchURL("https://[2001:4860:4860::8888]/v1/models"); err != nil {
		t.Errorf("公网 IPv6 不应被拦截：%v", err)
	}
}

// 主机名判定必须覆盖「绝对形式域名」与 SECURITY.md 承诺的保留地址段：
// 只靠 net.ParseIP 会漏掉 localhost.、运营商级 NAT、测试网、6to4 等。
func TestIsPrivateOrReservedHostCoverage(t *testing.T) {
	blocked := []string{
		"localhost", "localhost.", "LOCALHOST.", "api.localhost", "api.localhost.",
		"printer.local", "printer.local.",
		"127.0.0.1", "127.0.0.1.", "0.0.0.0", "10.1.2.3", "172.16.0.9", "192.168.1.1",
		"169.254.1.1", "100.64.0.1", "192.0.2.5", "192.88.99.1", "198.18.0.1",
		"198.51.100.9", "203.0.113.9", "224.0.0.1", "239.255.255.250", "240.0.0.1",
		"255.255.255.255",
		"::1", "fe80::1", "fd00::1", "fc00::1", "ff02::1", "2001:db8::1",
		"2002:0a00:0001::1", "64:ff9b::1", "100::1", "::ffff:127.0.0.1",
	}
	for _, host := range blocked {
		if !isPrivateOrReservedHost(host) {
			t.Errorf("%q 应按私有/保留地址拦截", host)
		}
	}
	allowed := []string{"api.openai.com", "example.invalid", "8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"}
	for _, host := range allowed {
		if isPrivateOrReservedHost(host) {
			t.Errorf("%q 是公网主机，不应被拦截", host)
		}
	}
}

// URL 层面的入口也要挡住绝对形式域名（net/url 会把它原样留在 Hostname 里）。
func TestValidateFetchURLBlocksAbsoluteHostName(t *testing.T) {
	prevPrivate := PrivateHostsAllowed()
	SetPrivateHostsAllowed(false)
	t.Cleanup(func() { SetPrivateHostsAllowed(prevPrivate) })
	if err := ValidateFetchURL("https://localhost./v1/models"); err == nil {
		t.Error("https://localhost./v1/models 应按私有地址拦截")
	}
}

func TestSameURLOrigin(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://api.example.invalid/v1", "https://api.example.invalid/openai/v1", true},
		{"https://API.Example.invalid/v1", "https://api.example.invalid", true},
		{"https://api.example.invalid:443/v1", "https://api.example.invalid/v1", true},
		{"http://api.example.invalid:80/v1", "http://api.example.invalid/v1", true},
		{"https://api.example.invalid/v1", "https://evil.example.invalid/v1", false},
		{"https://api.example.invalid/v1", "http://api.example.invalid/v1", false},
		{"https://api.example.invalid/v1", "https://api.example.invalid:8443/v1", false},
		{"https://api.example.invalid/v1", "", false},
		{"", "", false},
		{"https://api.example.invalid/v1", "://bad url", false},
	}
	for _, c := range cases {
		if got := SameURLOrigin(c.a, c.b); got != c.want {
			t.Errorf("SameURLOrigin(%q, %q) = %v，期望 %v", c.a, c.b, got, c.want)
		}
	}
}
