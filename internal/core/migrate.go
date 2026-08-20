package core

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// MigrateSide holds preview info for one agent side.
type MigrateSide struct {
	Agent     string            `json:"agent"`
	Label     string            `json:"label"`
	Path      string            `json:"path"`
	Exists    bool              `json:"exists"`
	Error     string            `json:"error"`
	Providers []ProviderSummary `json:"providers"`
	RawText   string            `json:"raw_text"`
	Format    string            `json:"format"`
}

// MigratePreview aggregates source and target sides.
type MigratePreview struct {
	Source MigrateSide `json:"source"`
	Target MigrateSide `json:"target"`
}

func agentLabel(id string) string {
	for _, a := range AllAgents() {
		if string(a.ID) == NormalizeAgentID(id).String() {
			return a.Label
		}
	}
	return id
}

func (a AgentID) String() string { return string(a) }

func readRawText(path string) (string, string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	if isYAMLPath(path) {
		// Normalize via yaml round-trip for readable display, but keep original if small
		if len(data) > 200*1024 {
			return string(data[:200*1024]) + "\n... (truncated)", "yaml"
		}
		return string(data), "yaml"
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "{}", "json"
	}
	// Try pretty print JSONC -> JSON
	cleaned := StripJSONCComments(text)
	var v interface{}
	if err := json.Unmarshal([]byte(cleaned), &v); err == nil {
		pretty, _ := json.MarshalIndent(v, "", "  ")
		if len(pretty) > 200*1024 {
			return string(pretty[:200*1024]) + "\n... (truncated)", "json"
		}
		return string(pretty), "json"
	}
	if len(data) > 200*1024 {
		return string(data[:200*1024]) + "\n... (truncated)", "json"
	}
	return string(data), "json"
}

func buildSide(agentID string) MigrateSide {
	norm := string(NormalizeAgentID(agentID))
	label := agentLabel(norm)
	path := AgentDefaultPath(AgentID(norm))
	side := MigrateSide{Agent: norm, Label: label, Path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		side.Exists = false
		side.Providers = []ProviderSummary{}
		side.RawText = ""
		side.Format = "json"
		if isYAMLPath(path) {
			side.Format = "yaml"
		}
		return side
	}
	side.Exists = true
	side.Format = "json"
	if isYAMLPath(path) {
		side.Format = "yaml"
	}
	preview := ConfigProviderSummaryForAgent(path, norm)
	side.Error = preview.Error
	if preview.Error == "" {
		side.Providers = preview.Providers
		if side.Providers == nil {
			side.Providers = []ProviderSummary{}
		}
	} else {
		side.Providers = []ProviderSummary{}
	}
	raw, fmtStr := readRawText(path)
	side.RawText = raw
	side.Format = fmtStr
	if side.Format == "" {
		side.Format = "json"
	}
	return side
}

// MigratePreviewForAgents builds preview for source and target.
func MigratePreviewForAgents(sourceAgent, targetAgent string) MigratePreview {
	if strings.TrimSpace(sourceAgent) == "" {
		sourceAgent = string(AgentZCode)
	}
	if strings.TrimSpace(targetAgent) == "" {
		targetAgent = string(AgentOpenCode)
	}
	return MigratePreview{
		Source: buildSide(sourceAgent),
		Target: buildSide(targetAgent),
	}
}

// MigrateExecute performs migration from source to target.
func MigrateExecute(sourceAgent, targetAgent string, selectedIDs []string, mode string) (map[string]interface{}, error) {
	srcNorm := string(NormalizeAgentID(sourceAgent))
	tgtNorm := string(NormalizeAgentID(targetAgent))
	if srcNorm == tgtNorm {
		return nil, fmt.Errorf("源和目标不能是同一个 Agent")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "overwrite" && mode != "merge" {
		mode = "merge"
	}
	// Dedup selectedIDs
	seen := map[string]bool{}
	var deduped []string
	for _, id := range selectedIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		deduped = append(deduped, id)
	}
	if len(deduped) == 0 {
		return nil, fmt.Errorf("请至少选择一个提供商进行迁移")
	}
	// Load source config and collect providers to migrate
	srcPath := AgentDefaultPath(AgentID(srcNorm))
	var srcProviders map[string]interface{}
	var srcLoadErr error
	if srcNorm == string(AgentDeepSeek) {
		cfg, err := DeepSeekLoadConfig(srcPath)
		if err != nil {
			return nil, fmt.Errorf("读取源配置失败：%v", err)
		}
		srcProviders = deepSeekProvidersMap(cfg)
		if srcProviders == nil {
			srcProviders = map[string]interface{}{}
		}
		// Keep raw cfg for reference if needed but not used
		_ = cfg
	} else {
		cfg, err := LoadConfig(srcPath)
		if err != nil {
			return nil, fmt.Errorf("读取源配置失败：%v", err)
		}
		srcProviders, _ = cfg["provider"].(map[string]interface{})
		if srcProviders == nil {
			srcProviders = map[string]interface{}{}
		}
		srcLoadErr = nil
		_ = srcLoadErr
	}
	// Validate all selected exist
	for _, pid := range deduped {
		if _, ok := srcProviders[pid]; !ok {
			return nil, fmt.Errorf("源配置中不存在提供商「%s」", pid)
		}
	}
	// Convert each selected to normalized providerCfg (zcode normalized)
	type migrated struct {
		pid string
		cfg map[string]interface{}
	}
	var toMigrate []migrated
	for _, pid := range deduped {
		raw := srcProviders[pid]
		m, _ := raw.(map[string]interface{})
		if m == nil {
			m = map[string]interface{}{}
		}
		var newPID string
		var cfg map[string]interface{}
		var err error
		if srcNorm == string(AgentDeepSeek) {
			newPID, cfg, err = ConvertDeepSeekProvider(pid, m, nil)
		} else if srcNorm == string(AgentOpenCode) {
			// Opencode may have subtle differences but Sanitize works; try ConvertOpencodeProvider for fidelity
			newPID, cfg, err = ConvertOpencodeProvider(pid, m, nil)
			if err != nil {
				newPID, cfg, err = SanitizeZCodeProvider(pid, m)
			}
		} else {
			newPID, cfg, err = SanitizeZCodeProvider(pid, m)
		}
		if err != nil {
			return nil, fmt.Errorf("转换提供商「%s」失败：%v", pid, err)
		}
		toMigrate = append(toMigrate, migrated{pid: newPID, cfg: cfg})
	}
	// Load target config
	tgtPath := AgentDefaultPath(AgentID(tgtNorm))
	if tgtNorm == string(AgentDeepSeek) {
		cfg, err := DeepSeekLoadConfig(tgtPath)
		if err != nil {
			return nil, fmt.Errorf("读取目标配置失败：%v", err)
		}
		if cfg == nil {
			cfg = map[string]interface{}{}
		}
		// Fingerprint before modification
		fp, _ := FileFingerprint(tgtPath)
		// Ensure backup
		bak, _ := BackupConfig(tgtPath)
		// Determine llm mode
		llmMode := isDeepSeekLLMMode(cfg)
		// If file was empty and not llmMode, decide to use llmMode for future? Keep as is.
		// For overwrite, clear providers
		if llmMode {
			llm, _ := cfg["llm-pi-ai"].(map[string]interface{})
			if llm == nil {
				llm = map[string]interface{}{}
				cfg["llm-pi-ai"] = llm
			}
			providers, _ := llm["providers"].(map[string]interface{})
			if providers == nil {
				providers = map[string]interface{}{}
				llm["providers"] = providers
			}
			if mode == "overwrite" {
				for k := range providers {
					delete(providers, k)
				}
			}
			// Insert each migrated
			for _, item := range toMigrate {
				opts, _ := item.cfg["options"].(map[string]interface{})
				baseURL, _ := opts["baseURL"].(string)
				apiKey, _ := opts["apiKey"].(string)
				kindStr, _ := item.cfg["kind"].(string)
				uiKind := ConfigKindToUIKind(kindStr, baseURL)
				modelsMap, _ := item.cfg["models"].(map[string]interface{})
				var arr []interface{}
				// sort model ids for stable output
				var mids []string
				for mid := range modelsMap {
					mids = append(mids, mid)
				}
				sort.Strings(mids)
				for _, mid := range mids {
					mcfg, _ := modelsMap[mid].(map[string]interface{})
					card := CfgToCard(mid, mcfg)
					arr = append(arr, dshModelToRaw(card, uiKind))
				}
				entry := map[string]interface{}{}
				if existing, ok := providers[item.pid].(map[string]interface{}); ok && mode == "merge" {
					entry = deepCopyMap(existing)
				}
				entry["displayName"] = item.cfg["name"]
				entry["baseURL"] = baseURL
				if strings.TrimSpace(apiKey) != "" {
					envName := strings.ToUpper(strings.ReplaceAll(item.pid, "-", "_")) + "_API_KEY"
					// also replace colon
					envName = strings.ReplaceAll(envName, ":", "_")
					entry["apiKeyEnv"] = envName
					if err := saveDeepSeekCredential(envName, apiKey); err != nil {
						return nil, fmt.Errorf("写入 DeepSeek 凭证失败：%v", err)
					}
					delete(entry, "apiKey")
				} else {
					// keep existing apiKeyEnv if merge and no key provided
					if _, ok := entry["apiKeyEnv"]; !ok {
						delete(entry, "apiKey")
					}
				}
				if uiKind == "anthropic" {
					entry["api"] = "anthropic"
				} else {
					entry["api"] = "openai-completions"
				}
				if arr == nil {
					arr = []interface{}{}
				}
				entry["models"] = arr
				providers[item.pid] = entry
			}
		} else {
			// Legacy provider mode for deepseek (uses "provider" key)
			providers, _ := cfg["provider"].(map[string]interface{})
			if providers == nil {
				providers = map[string]interface{}{}
				cfg["provider"] = providers
			}
			if mode == "overwrite" {
				for k := range providers {
					delete(providers, k)
				}
			}
			for _, item := range toMigrate {
				if _, exists := providers[item.pid]; exists && mode == "merge" {
					mergedCfg, err := MergeProviderIntoConfig(map[string]interface{}{"provider": map[string]interface{}{item.pid: providers[item.pid]}}, item.pid, item.cfg, true)
					if err != nil {
						return nil, err
					}
					providers[item.pid] = mergedCfg["provider"].(map[string]interface{})[item.pid]
				} else {
					providers[item.pid] = deepCopyMap(item.cfg)
				}
			}
		}
		if err := writeDeepSeekConfig(tgtPath, cfg, fp); err != nil {
			return nil, fmt.Errorf("写入目标配置失败：%v", err)
		}
		latestBak := bak
		if latestBak == "" {
			latestBak = FindLatestBackup(tgtPath)
		}
		return map[string]interface{}{
			"success": true, "migrated": len(toMigrate), "mode": mode,
			"source": srcNorm, "target": tgtNorm, "target_path": tgtPath,
			"backup": latestBak,
		}, nil
	}
	// Non-deepseek target (zcode / opencode) : JSON provider map
	cfg, fingerprint, err := LoadConfigWithFingerprint(tgtPath)
	if err != nil {
		return nil, fmt.Errorf("读取目标配置失败：%v", err)
	}
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	NormalizeConfigKinds(cfg)
	bak, _ := BackupConfig(tgtPath)
	if mode == "overwrite" {
		// Keep non-provider top-level keys, replace provider map with migrated set
		newProviders := map[string]interface{}{}
		for _, item := range toMigrate {
			newProviders[item.pid] = deepCopyMap(item.cfg)
		}
		cfg["provider"] = newProviders
		if err := WriteConfig(tgtPath, cfg, fingerprint); err != nil {
			return nil, fmt.Errorf("写入目标配置失败：%v", err)
		}
	} else {
		// merge mode
		for _, item := range toMigrate {
			merged, err := MergeProviderIntoConfig(cfg, item.pid, item.cfg, true)
			if err != nil {
				return nil, err
			}
			cfg = merged
		}
		if err := WriteConfig(tgtPath, cfg, fingerprint); err != nil {
			return nil, fmt.Errorf("写入目标配置失败：%v", err)
		}
	}
	latestBak := bak
	if latestBak == "" {
		latestBak = FindLatestBackup(tgtPath)
	}
	return map[string]interface{}{
		"success": true, "migrated": len(toMigrate), "mode": mode,
		"source": srcNorm, "target": tgtNorm, "target_path": tgtPath,
		"backup": latestBak,
	}, nil
}
