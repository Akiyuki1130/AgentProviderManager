package core

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// apm-backup 信封的加解密测试：只用占位 Key，环境全部沙箱化。

func backupFixtureProviders() []ProviderEdit {
	return []ProviderEdit{
		{
			ID: "acme", Name: "Acme", Kind: "openai-compatible",
			BaseURL: "https://acme.example.invalid/v1", APIKey: "sk-placeholder",
			OptionsJSON: `{"apiKey":"sk-placeholder","headers":{"X-Test":"1"}}`,
			Cards:       []ModelCard{{ModelID: "m1"}},
			RawProvider: map[string]interface{}{"name": "Acme"},
		},
		{
			ID: "beta", Name: "Beta", Kind: "anthropic",
			BaseURL: "https://beta.example.invalid", APIKey: "sk-placeholder-2",
			OptionsJSON: `{"apiKey":"sk-placeholder-2","baseURL":"https://beta.example.invalid"}`,
		},
	}
}

func TestBackupPasswordRoundTrip(t *testing.T) {
	pkg := BackupPackage{Agent: "zcode", IncludeKeys: true, KeyMode: "password", Providers: backupFixtureProviders()}
	raw, err := SealBackup(pkg, "pw123")
	if err != nil {
		t.Fatalf("SealBackup 失败：%v", err)
	}
	if bytes.Contains(raw, []byte("sk-placeholder")) {
		t.Fatal("信封泄露了明文 API Key")
	}
	if _, err := OpenBackup(raw, ""); !errors.Is(err, ErrBackupNeedsPassword) {
		t.Fatalf("空密码应返回 ErrBackupNeedsPassword，实际：%v", err)
	}
	if _, err := OpenBackup(raw, "wrong-pw"); !errors.Is(err, ErrBackupWrongPassword) {
		t.Fatalf("错误密码应返回 ErrBackupWrongPassword，实际：%v", err)
	}
	got, err := OpenBackup(raw, "pw123")
	if err != nil {
		t.Fatalf("OpenBackup 失败：%v", err)
	}
	if !reflect.DeepEqual(got.Providers, pkg.Providers) {
		t.Errorf("解密后的 provider 不一致：%+v", got.Providers)
	}
}

func TestStripBackupKeys(t *testing.T) {
	in := backupFixtureProviders()
	out := StripBackupKeys(in)
	if in[0].APIKey != "sk-placeholder" {
		t.Error("StripBackupKeys 不应修改输入切片")
	}
	if out[0].APIKey != "" || out[1].APIKey != "" {
		t.Error("APIKey 应被清空")
	}
	if out[0].RawProvider != nil {
		t.Error("RawProvider 应被置空")
	}
	if strings.Contains(out[0].OptionsJSON, "apiKey") {
		t.Errorf("OptionsJSON 仍含 apiKey：%s", out[0].OptionsJSON)
	}
	if !strings.Contains(out[0].OptionsJSON, "X-Test") {
		t.Errorf("非敏感 Options 应保留：%s", out[0].OptionsJSON)
	}
}

func TestBackupSealedRoundTrip(t *testing.T) {
	pkg := BackupPackage{Agent: "zcode", IncludeKeys: true, KeyMode: "sealed", Providers: backupFixtureProviders()}
	raw, err := SealBackup(pkg, "")
	if err != nil {
		t.Fatalf("SealBackup(sealed) 失败：%v", err)
	}
	if bytes.Contains(raw, []byte("sk-placeholder")) {
		t.Fatal("sealed 信封泄露了明文 API Key")
	}
	got, err := OpenBackup(raw, "")
	if err != nil {
		t.Fatalf("OpenBackup(sealed) 失败：%v", err)
	}
	if !reflect.DeepEqual(got.Providers, pkg.Providers) {
		t.Errorf("sealed 解密后的 provider 不一致：%+v", got.Providers)
	}
}
