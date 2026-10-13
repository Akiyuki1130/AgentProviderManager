package app

import (
	"testing"

	"agentprovidermanager/internal/core"
)

// keychain_links 绑定的持久化测试：只用占位 ID，环境沙箱化。

func TestKeychainLinkPersistence(t *testing.T) {
	a := sandboxApp(t)
	a.currentAgent = string(core.AgentZCode)
	if resp := a.SetKeychainLink("", "entry"); resp["success"] == true {
		t.Error("空 providerID 应被拒绝")
	}
	requireSuccess(t, a.SetKeychainLink("acme", "entry-1"), "SetKeychainLink")
	resp := a.GetKeychainLinks()
	requireSuccess(t, resp, "GetKeychainLinks")
	links, _ := resp["links"].(map[string]interface{})
	if links["zcode|acme"] != "entry-1" {
		t.Fatalf("关联未写入：%v", links)
	}
	requireSuccess(t, a.SetKeychainLink("acme", "entry-2"), "替换关联")

	// 新实例重读同一份沙箱 settings，验证持久化。
	b := NewApp()
	resp = b.GetKeychainLinks()
	links, _ = resp["links"].(map[string]interface{})
	if links["zcode|acme"] != "entry-2" || len(links) != 1 {
		t.Fatalf("重读后关联不正确：%v", links)
	}

	requireSuccess(t, a.RemoveKeychainLink("acme"), "RemoveKeychainLink")
	resp = b.GetKeychainLinks()
	links, _ = resp["links"].(map[string]interface{})
	if len(links) != 0 {
		t.Fatalf("删除后应不再有该关联：%v", links)
	}
}
