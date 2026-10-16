package app

import (
	"testing"

	"agentprovidermanager/internal/core"
)

// keychain_links 绑定的持久化测试：只用占位 ID，环境沙箱化。

func saveKeychainEntry(t *testing.T, a *App, name, key string) string {
	t.Helper()
	resp := a.SaveKeychainEntry(map[string]interface{}{"name": name, "api_key": key})
	requireSuccess(t, resp, "SaveKeychainEntry")
	entry, _ := resp["entry"].(map[string]interface{})
	id, _ := entry["id"].(string)
	if id == "" {
		t.Fatalf("保存钥匙串条目未返回 ID：%v", resp)
	}
	return id
}

func TestKeychainLinkPersistence(t *testing.T) {
	a := sandboxApp(t)
	a.currentAgent = string(core.AgentZCode)
	if resp := a.SetKeychainLink("", "entry"); resp["success"] == true {
		t.Error("空 providerID 应被拒绝")
	}
	// 指向不存在的条目会让钥匙串页显示成「已关联但填不出 Key」，必须直接拒绝。
	if resp := a.SetKeychainLink("acme", "entry-missing"); resp["success"] == true {
		t.Error("不存在的钥匙串条目应被拒绝")
	}
	entry1 := saveKeychainEntry(t, a, "Acme Key", "sk-placeholder-1")
	entry2 := saveKeychainEntry(t, a, "Acme Key 2", "sk-placeholder-2")

	requireSuccess(t, a.SetKeychainLink("acme", entry1), "SetKeychainLink")
	resp := a.GetKeychainLinks()
	requireSuccess(t, resp, "GetKeychainLinks")
	links, _ := resp["links"].(map[string]interface{})
	if links["zcode|acme"] != entry1 {
		t.Fatalf("关联未写入：%v", links)
	}
	requireSuccess(t, a.SetKeychainLink("acme", entry2), "替换关联")

	// 新实例重读同一份沙箱 settings，验证持久化。
	b := NewApp()
	resp = b.GetKeychainLinks()
	links, _ = resp["links"].(map[string]interface{})
	if links["zcode|acme"] != entry2 || len(links) != 1 {
		t.Fatalf("重读后关联不正确：%v", links)
	}

	requireSuccess(t, a.RemoveKeychainLink("acme"), "RemoveKeychainLink")
	resp = b.GetKeychainLinks()
	links, _ = resp["links"].(map[string]interface{})
	if len(links) != 0 {
		t.Fatalf("删除后应不再有该关联：%v", links)
	}
}
