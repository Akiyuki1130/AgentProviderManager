package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/zcodeprovider"
)

// 统一写盘闸门：任何把 ZCode 新版文档写坏的旧式写法（读成普通 map -> 塞旧版顶层
// provider -> 整体写回）都必须在写入前失败。否则 ZCode 的 .strict() 校验会失败，
// 它会把整份个人配置当成空，所有供应商在使用端消失。
func TestWriteConfigRefusesToCorruptZCodeV2Doc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provider_config.json")

	valid, err := zcodeprovider.Encode(zcodeprovider.NewConfig())
	if err != nil {
		t.Fatalf("构造合法新版配置失败：%v", err)
	}
	if err := os.WriteFile(path, valid, 0644); err != nil {
		t.Fatalf("写入初始配置失败：%v", err)
	}

	doc := map[string]interface{}{
		"schemaVersion": 1,
		"config": map[string]interface{}{
			"providerConfigRules": map[string]interface{}{"providerRules": []interface{}{}},
			"modelConfigRules": map[string]interface{}{
				"providerModelRules":       []interface{}{},
				"manualProviderModelRules": []interface{}{},
			},
		},
		// 旧版字段被误合并进新版文档：ZCode 会拒绝整份配置。
		"provider": map[string]interface{}{"acme": map[string]interface{}{"name": "Acme"}},
	}
	err = WriteConfig(path, doc, "")
	if err == nil {
		t.Fatal("应拒绝写出含未知顶层键 provider 的新版文档")
	}
	if !strings.Contains(err.Error(), "拒绝写入") {
		t.Errorf("错误信息应说明拒绝原因，实际：%v", err)
	}
	got, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatalf("读取目标文件失败：%v", rerr)
	}
	if string(got) != string(valid) {
		t.Error("拒绝写入时不应改动磁盘上的文件")
	}
}

// 闸门不能误伤其它 agent：旧版 ZCode / OpenCode 形状的 JSON 仍然照常写出。
func TestWriteConfigStillWritesLegacyJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := map[string]interface{}{
		"provider": map[string]interface{}{
			"acme": map[string]interface{}{"name": "Acme", "kind": "openai-compatible"},
		},
	}
	if err := WriteConfig(path, legacy, ""); err != nil {
		t.Fatalf("旧版形状不应被闸门拦截：%v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	if !strings.Contains(string(got), "Acme") {
		t.Error("旧版配置应已写出")
	}
}

// 合法的新版文档（由 zcodeprovider.Encode 产出）必须照常写出：闸门只拦坏文档。
func TestWriteConfigAcceptsValidZCodeV2Doc(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider_config.json")
	data, err := zcodeprovider.Encode(zcodeprovider.NewConfig())
	if err != nil {
		t.Fatalf("编码失败：%v", err)
	}
	if err := WriteConfigBytes(path, data, ""); err != nil {
		t.Fatalf("合法新版文档不应被闸门拦截：%v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	if string(got) != string(data) {
		t.Error("写出内容应与输入一致")
	}
}
