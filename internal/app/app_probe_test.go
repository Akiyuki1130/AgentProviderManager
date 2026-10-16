package app

import "testing"

// 「测试连接」沿用本地已存 Key 的同源闸门：只测纯判定函数，不建连、不发请求。
// 覆盖修复的缺口——把 Base URL 覆盖成任意主机后，不得再把本地真实 Key 发过去。

func TestAcceptsLocalKeyForOverride(t *testing.T) {
	tests := []struct {
		name         string
		localBaseURL string
		override     string
		want         bool
	}{
		{"无覆盖时沿用本地 Key", "https://acme.example.invalid/v1", "", true},
		{"同源仅路径不同时沿用", "https://acme.example.invalid/v1", "https://acme.example.invalid/other", true},
		{"同源带默认端口时沿用", "https://acme.example.invalid/v1", "https://acme.example.invalid:443/v2", true},
		{"主机不同不沿用", "https://acme.example.invalid/v1", "https://evil.example.invalid/v1", false},
		{"端口不同不沿用", "https://acme.example.invalid/v1", "https://acme.example.invalid:8443/v1", false},
		{"协议不同不沿用", "https://acme.example.invalid/v1", "http://acme.example.invalid/v1", false},
		{"覆盖值无法解析不沿用", "https://acme.example.invalid/v1", "://nope", false},
		{"本地 Base URL 缺失不沿用", "", "https://evil.example.invalid/v1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acceptsLocalKeyForOverride(tt.localBaseURL, tt.override); got != tt.want {
				t.Fatalf("acceptsLocalKeyForOverride(%q, %q) = %v，期望 %v",
					tt.localBaseURL, tt.override, got, tt.want)
			}
		})
	}
}
