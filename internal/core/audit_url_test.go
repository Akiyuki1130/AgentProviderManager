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
