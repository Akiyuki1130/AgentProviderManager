package core

import (
	"bytes"
	"encoding/json"
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
	in := append(backupFixtureProviders(), ProviderEdit{
		ID: "gamma", Name: "Gamma", Kind: "openai-compatible",
		BaseURL: "https://gamma.example.invalid/v1", APIKey: "sk-placeholder-3",
		Cards: []ModelCard{{
			ModelID: "gpt-placeholder",
			Headers: map[string]string{
				"Authorization": "Bearer sk-placeholder-3",
				"X-Api-Key":     "sk-placeholder-3",
				"x-custom":      "keep",
			},
			Options: map[string]interface{}{
				"apiKey":  "sk-placeholder-3",
				"nested":  map[string]interface{}{"access_token": "sk-placeholder-3", "note": "keep"},
				"baseURL": "https://gamma.example.invalid/v1",
			},
			RawCfg: map[string]interface{}{
				"apiKey":  "sk-placeholder-3",
				"headers": map[string]interface{}{"Authorization": "Bearer sk-placeholder-3", "x-custom": "keep"},
				"options": map[string]interface{}{"apiKey": "sk-placeholder-3", "note": "keep"},
				"baseURL": "https://gamma.example.invalid/v1",
			},
		}},
	})
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

	// 卡片级凭据：模型条目自带的 headers / options / _raw_cfg 都是候选，
	// 只清 provider 层会让「不含密钥」备份照样外流。
	gamma := out[2].Cards[0]
	if _, ok := gamma.Headers["Authorization"]; ok {
		t.Error("Cards[0].Headers 的 Authorization 应被删除")
	}
	if _, ok := gamma.Headers["X-Api-Key"]; ok {
		t.Error("Cards[0].Headers 的 X-Api-Key 应被删除")
	}
	if gamma.Headers["x-custom"] != "keep" {
		t.Errorf("Cards[0].Headers 的非敏感项应保留：%v", gamma.Headers)
	}
	if _, ok := gamma.Options["apiKey"]; ok {
		t.Error("Cards[0].Options 的 apiKey 应被删除")
	}
	nested, _ := gamma.Options["nested"].(map[string]interface{})
	if _, ok := nested["access_token"]; ok {
		t.Errorf("Cards[0].Options 深层的 access_token 应被删除：%v", gamma.Options)
	}
	if nested["note"] != "keep" {
		t.Errorf("Cards[0].Options.nested 的非敏感项应保留：%v", nested)
	}
	if gamma.Options["baseURL"] != "https://gamma.example.invalid/v1" {
		t.Errorf("Cards[0].Options 的 baseURL 应保留：%v", gamma.Options)
	}
	if _, ok := gamma.RawCfg["apiKey"]; ok {
		t.Error("Cards[0].RawCfg 的 apiKey 应被删除")
	}
	rawHeaders, _ := gamma.RawCfg["headers"].(map[string]interface{})
	if _, ok := rawHeaders["Authorization"]; ok {
		t.Errorf("Cards[0].RawCfg.headers 的 Authorization 应被删除：%v", gamma.RawCfg)
	}
	if rawHeaders["x-custom"] != "keep" {
		t.Errorf("Cards[0].RawCfg.headers 的非敏感项应保留：%v", rawHeaders)
	}
	if gamma.RawCfg["baseURL"] != "https://gamma.example.invalid/v1" {
		t.Errorf("Cards[0].RawCfg 的 baseURL 应保留：%v", gamma.RawCfg)
	}
	if gamma.ModelID != "gpt-placeholder" {
		t.Errorf("模型名应保留，实际：%q", gamma.ModelID)
	}
	// 兜底：整张卡片序列化后不得再出现占位 Key。
	if b, err := json.Marshal(out[2]); err == nil && strings.Contains(string(b), "sk-placeholder-3") {
		t.Errorf("剥离后卡片仍含凭据：%s", b)
	}
	// 输入不得被改动：Cards 里的 map 是先深拷贝再剥离的。
	if _, ok := in[2].Cards[0].Headers["Authorization"]; !ok {
		t.Error("StripBackupKeys 不应修改输入卡片的 Headers")
	}
	if _, ok := in[2].Cards[0].Options["apiKey"]; !ok {
		t.Error("StripBackupKeys 不应修改输入卡片的 Options")
	}
	if _, ok := in[2].Cards[0].RawCfg["apiKey"]; !ok {
		t.Error("StripBackupKeys 不应修改输入卡片的 RawCfg")
	}
}

// 敏感键的清理必须递归且忽略大小写与 - _ 分隔符：headers 里的 Authorization /
// X-Api-Key 会随「不含密钥」备份一起外流，只删顶层 apiKey 是漏的。
func TestStripBackupKeysRecursesAndFoldsCase(t *testing.T) {
	in := []ProviderEdit{{
		ID: "acme", APIKey: "sk-placeholder",
		OptionsJSON: `{"headers":{"Authorization":"Bearer sk-placeholder","X-Api-Key":"sk-placeholder","x-custom":"keep"},"nested":{"deep":[{"access_token":"sk-placeholder","note":"keep"}]},"baseURL":"https://acme.example.invalid"}`,
	}}
	out := StripBackupKeys(in)
	if strings.Contains(out[0].OptionsJSON, "sk-placeholder") {
		t.Errorf("凭据仍在 OptionsJSON 里：%s", out[0].OptionsJSON)
	}
	for _, keep := range []string{"x-custom", "note", "baseURL"} {
		if !strings.Contains(out[0].OptionsJSON, keep) {
			t.Errorf("非敏感键 %q 应保留：%s", keep, out[0].OptionsJSON)
		}
	}
}

// 迭代数来自文件，必须有上界：否则构造 iters 极大的备份即可让解密长时间占住线程。
func TestOpenBackupRejectsOversizedPBKDF2Iters(t *testing.T) {
	raw, err := json.Marshal(BackupPackage{
		Format: BackupFormat, Version: BackupVersion, KeyMode: "password",
		Enc: &BackupEncBlock{
			Algo: "aes-256-gcm", KDF: "pbkdf2-sha256",
			Salt:  "c2FsdHNhbHRzYWx0c2Ex", // "saltsaltsaltsa1"
			Iters: backupPBKDF2ItersMax + 1,
			Nonce: "bm9uY2Vub25jZTEy", Data: "ZGF0YQ==",
		},
	})
	if err != nil {
		t.Fatalf("构造信封失败：%v", err)
	}
	if _, err := OpenBackup(raw, "pw123"); !errors.Is(err, ErrBackupUnsupportedParams) {
		t.Fatalf("超范围迭代数应返回 ErrBackupUnsupportedParams，实际：%v", err)
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
