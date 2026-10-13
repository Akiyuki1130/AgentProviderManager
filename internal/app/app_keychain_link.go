package app

import "strings"

// keychain_links 在 settings.json 里记录「agent|providerID」-> 钥匙串条目 ID 的映射，
// 用于把供应商关联到钥匙串里的凭据条目。

func keychainLinkKey(agent, providerID string) string { return agent + "|" + providerID }

func keychainLinksFrom(s map[string]interface{}) map[string]interface{} {
	links, _ := s["keychain_links"].(map[string]interface{})
	if links == nil {
		links = map[string]interface{}{}
	}
	return links
}

func (a *App) GetKeychainLinks() map[string]interface{} {
	return map[string]interface{}{"success": true, "links": keychainLinksFrom(a.loadSettings())}
}

func (a *App) SetKeychainLink(providerID, entryID string) map[string]interface{} {
	providerID = strings.TrimSpace(providerID)
	entryID = strings.TrimSpace(entryID)
	if providerID == "" || entryID == "" {
		return map[string]interface{}{"success": false, "error": "参数不能为空"}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.loadSettings()
	links := keychainLinksFrom(s)
	links[keychainLinkKey(a.currentAgent, providerID)] = entryID
	s["keychain_links"] = links
	if !a.saveSettings(s) {
		return map[string]interface{}{"success": false, "error": "设置保存失败"}
	}
	return map[string]interface{}{"success": true}
}

func (a *App) RemoveKeychainLink(providerID string) map[string]interface{} {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return map[string]interface{}{"success": false, "error": "参数不能为空"}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.loadSettings()
	links := keychainLinksFrom(s)
	delete(links, keychainLinkKey(a.currentAgent, providerID))
	s["keychain_links"] = links
	if !a.saveSettings(s) {
		return map[string]interface{}{"success": false, "error": "设置保存失败"}
	}
	return map[string]interface{}{"success": true}
}
