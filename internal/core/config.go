package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// LoadConfig loads a JSON/JSONC config file. Missing file returns empty map.
func LoadConfig(path string) (map[string]interface{}, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigParseError{Msg: fmt.Sprintf("读取配置文件失败：%v", err)}
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	cleaned := StripJSONCComments(string(data))
	var result interface{}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, &ConfigParseError{Msg: fmt.Sprintf("配置文件解析失败（JSON 格式错误）：%v。为避免覆盖现有配置，已中止操作。", err)}
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		return nil, &ConfigParseError{Msg: "配置文件根节点必须是对象 {...}，已中止操作"}
	}
	return m, nil
}

// StripJSONCComments removes // and /* */ comments and trailing commas.
func StripJSONCComments(text string) string {
	var out []rune
	runes := []rune(text)
	n := len(runes)
	inString := false
	var stringChar rune
	i := 0
	for i < n {
		ch := runes[i]
		if inString {
			out = append(out, ch)
			if ch == '\\' && i+1 < n {
				out = append(out, runes[i+1])
				i += 2
				continue
			}
			if ch == stringChar {
				inString = false
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			inString = true
			stringChar = ch
			out = append(out, ch)
			i++
			continue
		}
		if ch == '/' && i+1 < n {
			nxt := runes[i+1]
			if nxt == '/' {
				i += 2
				for i < n && runes[i] != '\n' && runes[i] != '\r' {
					i++
				}
				continue
			}
			if nxt == '*' {
				i += 2
				for i < n && !(runes[i] == '*' && i+1 < n && runes[i+1] == '/') {
					i++
				}
				i += 2
				continue
			}
		}
		if ch == ',' {
			j := i + 1
			for j < n {
				for j < n && (runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\r' || runes[j] == '\n') {
					j++
				}
				if j+1 < n && runes[j] == '/' && runes[j+1] == '/' {
					j += 2
					for j < n && runes[j] != '\n' && runes[j] != '\r' {
						j++
					}
					continue
				}
				if j+1 < n && runes[j] == '/' && runes[j+1] == '*' {
					j += 2
					for j < n && !(runes[j] == '*' && j+1 < n && runes[j+1] == '/') {
						j++
					}
					j += 2
					continue
				}
				break
			}
			if j < n && (runes[j] == '}' || runes[j] == ']') {
				i++
				continue
			}
		}
		out = append(out, ch)
		i++
	}
	return string(out)
}

func NormalizeConfigKinds(config map[string]interface{}) bool {
	providers, ok := config["provider"].(map[string]interface{})
	if !ok {
		return false
	}
	valid := map[string]bool{"openai-compatible": true, "anthropic": true, "openai": true}
	changed := false
	for _, raw := range providers {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		kind, _ := m["kind"].(string)
		if kind == "responses" {
			m["kind"] = "openai"
			changed = true
			continue
		}
		trimmedKind := strings.TrimSpace(kind)
		if kind != trimmedKind && valid[trimmedKind] {
			m["kind"] = trimmedKind
			changed = true
			kind = trimmedKind
		}
		if kind != "" && valid[kind] {
			continue
		}
		if kind == "" || strings.TrimSpace(kind) == "" {
			continue
		}
		opts, _ := m["options"].(map[string]interface{})
		baseURL := ""
		if opts != nil {
			baseURL, _ = opts["baseURL"].(string)
		}
		fixed := KindToConfig[InferKind(baseURL)]
		m["kind"] = fixed
		changed = true
	}
	return changed
}

var backupRe = regexp.MustCompile(`\.bak_(\d{8}_\d{6}_\d{6})$`)

func backupPaths(configPath string) []string {
	if configPath == "" {
		return nil
	}
	dir := filepath.Dir(configPath)
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	base := filepath.Base(configPath)
	pattern := filepath.Join(dir, base+".bak_*")
	matches, _ := filepath.Glob(pattern)
	var baks []struct {
		path string
		ts   string
	}
	for _, p := range matches {
		m := backupRe.FindStringSubmatch(filepath.Base(p))
		if m == nil {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		baks = append(baks, struct {
			path string
			ts   string
		}{p, m[1]})
	}
	sort.Slice(baks, func(i, j int) bool { return baks[i].ts > baks[j].ts })
	out := make([]string, len(baks))
	for i, b := range baks {
		out[i] = b.path
	}
	return out
}

func ListBackups(configPath string) []string {
	all := backupPaths(configPath)
	if len(all) > MaxBackups {
		return all[:MaxBackups]
	}
	return all
}

func PruneBackups(configPath string) {
	all := backupPaths(configPath)
	if len(all) <= MaxBackups {
		return
	}
	for _, stale := range all[MaxBackups:] {
		_ = os.Remove(stale)
	}
}

func BackupConfig(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	now := time.Now()
	bak := fmt.Sprintf("%s.bak_%s", path, now.Format("20060102_150405_000000"))
	suffix := 1
	for {
		if _, err := os.Stat(bak); os.IsNotExist(err) {
			break
		}
		micro := (now.Nanosecond()/1000 + suffix) % 1_000_000
		bak = fmt.Sprintf("%s.bak_%s_%06d", path, now.Format("20060102_150405"), micro)
		suffix++
	}
	dir := filepath.Dir(bak)
	if dir == "" {
		dir = "."
	}
	tmpFile, err := os.CreateTemp(dir, filepath.Base(bak)+".tmp_*")
	if err != nil {
		return "", err
	}
	tmpName := tmpFile.Name()
	src, err := os.Open(path)
	if err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	_, err = tmpFile.ReadFrom(src)
	src.Close()
	if err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	_ = tmpFile.Sync()
	tmpFile.Close()
	if err := os.Rename(tmpName, bak); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	PruneBackups(path)
	return bak, nil
}

func MergeProviderIntoConfig(config map[string]interface{}, providerID string, providerCfg map[string]interface{}, mergeModels bool) (map[string]interface{}, error) {
	result := deepCopyMap(config)
	if _, ok := result["provider"]; !ok {
		result["provider"] = map[string]interface{}{}
	}
	providers, ok := result["provider"].(map[string]interface{})
	if !ok {
		return nil, &ConfigParseError{Msg: "配置文件中的 provider 字段格式错误（应为对象）"}
	}
	if _, exists := providers[providerID]; exists && mergeModels {
		existing, _ := providers[providerID].(map[string]interface{})
		if existing == nil {
			return nil, &ConfigParseError{Msg: fmt.Sprintf("提供商「%s」的配置格式错误（应为对象）", providerID)}
		}
		em, _ := existing["models"].(map[string]interface{})
		if em == nil {
			em = map[string]interface{}{}
		} else {
			em = deepCopyMap(em)
		}
		newModels, _ := providerCfg["models"].(map[string]interface{})
		if newModels == nil {
			newModels = map[string]interface{}{}
		}
		for mid, mcfg := range newModels {
			if emMid, ok := em[mid].(map[string]interface{}); ok {
				if newM, ok := mcfg.(map[string]interface{}); ok {
					merged := deepCopyMap(emMid)
					for k, v := range newM {
						merged[k] = deepCopyValue(v)
					}
					em[mid] = merged
				} else {
					em[mid] = deepCopyValue(mcfg)
				}
			} else {
				em[mid] = deepCopyValue(mcfg)
			}
		}
		existing["models"] = em
		eo, _ := existing["options"].(map[string]interface{})
		if eo == nil {
			eo = map[string]interface{}{}
		} else {
			eo = deepCopyMap(eo)
		}
		newOpts, _ := providerCfg["options"].(map[string]interface{})
		for k, v := range newOpts {
			eo[k] = v
		}
		existing["options"] = eo
		for _, k := range []string{"name", "kind", "source"} {
			if v, ok := providerCfg[k]; ok {
				existing[k] = v
			}
		}
		providers[providerID] = existing
	} else {
		providers[providerID] = deepCopyMap(providerCfg)
	}
	result["provider"] = providers
	return result, nil
}

func FileFingerprint(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d:%s", stat.ModTime().UnixNano(), stat.Size(), hex.EncodeToString(h[:])), nil
}

func LoadConfigForRestore(path string) (map[string]interface{}, error) {
	if isYAMLPath(path) {
		return DeepSeekLoadConfig(path)
	}
	return LoadConfig(path)
}

func LoadConfigWithFingerprint(path string) (map[string]interface{}, string, error) {
	before, _ := FileFingerprint(path)
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, "", err
	}
	after, _ := FileFingerprint(path)
	if before != after {
		return nil, "", fmt.Errorf("配置文件在读取期间被其他程序修改，请重试")
	}
	return cfg, after, nil
}

func WriteConfig(path string, config map[string]interface{}, expectedFingerprint string) error {
	dir := filepath.Dir(path)
	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	text, _ := json.MarshalIndent(config, "", "  ")
	text = append(text, '\n')
	d := dir
	if d == "" {
		d = "."
	}
	tmpFile, err := os.CreateTemp(d, filepath.Base(path)+".tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(text); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = tmpFile.Sync()
	tmpFile.Close()
	if expectedFingerprint != "" {
		cur, _ := FileFingerprint(path)
		if cur != expectedFingerprint {
			_ = os.Remove(tmpName)
			return fmt.Errorf("配置文件在操作期间被其他程序修改，请重试")
		}
	}
	return os.Rename(tmpName, path)
}

func FindLatestBackup(configPath string) string {
	baks := ListBackups(configPath)
	if len(baks) == 0 {
		return ""
	}
	return baks[0]
}

func CleanupStaleTmp(target string) int {
	if target == "" {
		return 0
	}
	dir := filepath.Dir(target)
	if _, err := os.Stat(dir); err != nil {
		return 0
	}
	base := filepath.Base(target)
	patterns := []string{
		base + ".tmp_*",
		base + ".restore_*",
		base + ".bak_*.tmp_*",
	}
	cutoff := time.Now().Add(-60 * time.Second)
	removed := 0
	for _, pat := range patterns {
		matches, _ := filepath.Glob(filepath.Join(dir, pat))
		for _, p := range matches {
			info, err := os.Stat(p)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if info.ModTime().After(cutoff) {
				continue
			}
			if err := os.Remove(p); err == nil {
				removed++
			}
		}
	}
	return removed
}

var snapshotRe = regexp.MustCompile(`\.snapshot_\d{8}_\d{6}_\d{6}$`)

func snapshotPaths(target string) []string {
	if target == "" {
		return nil
	}
	dir := filepath.Dir(target)
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join(dir, filepath.Base(target)+".snapshot_*"))
	var out []string
	for _, p := range matches {
		if !snapshotRe.MatchString(filepath.Base(p)) {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		out = append(out, p)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

func RestoreBackup(target, bakPath string) (string, error) {
	if _, err := LoadConfigForRestore(bakPath); err != nil {
		return "", err
	}
	targetFP, _ := FileFingerprint(target)
	var snap string
	if _, err := os.Stat(target); err == nil {
		snap = fmt.Sprintf("%s.snapshot_%s", target, time.Now().Format("20060102_150405_000000"))
		data, _ := os.ReadFile(target)
		_ = os.WriteFile(snap, data, 0644)
		cur, _ := FileFingerprint(target)
		if cur != targetFP {
			_ = os.Remove(snap)
			return "", fmt.Errorf("配置文件在恢复期间被其他程序修改，请重试")
		}
	}
	dir := filepath.Dir(target)
	if dir == "" {
		dir = "."
	}
	src, err := os.Open(bakPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	tmpFile, err := os.CreateTemp(dir, filepath.Base(target)+".restore_*")
	if err != nil {
		return "", err
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.ReadFrom(src); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	_ = tmpFile.Sync()
	tmpFile.Close()
	cur, _ := FileFingerprint(target)
	if cur != targetFP {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("配置文件在恢复期间被其他程序修改，请重试")
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	for _, stale := range snapshotPaths(target)[1:] {
		_ = os.Remove(stale)
	}
	return snap, nil
}

func RenameProviderInConfig(config map[string]interface{}, oldID, newID string) (bool, error) {
	newID = strings.TrimSpace(newID)
	if !ProviderIDValid(newID) {
		return false, fmt.Errorf("Provider ID 只能包含字母、数字、下划线、短横线或冒号（且不能为空）")
	}
	providers, _ := config["provider"].(map[string]interface{})
	if providers == nil || providers[oldID] == nil {
		return false, nil
	}
	if newID != oldID {
		if _, exists := providers[newID]; exists {
			return false, fmt.Errorf("提供商「%s」已存在，无法重命名", newID)
		}
	}
	providers[newID] = providers[oldID]
	delete(providers, oldID)
	return true, nil
}

func DetectConfigLocations() []ConfigLocation {
	return DetectConfigLocationsForAgent("")
}

func DetectConfigLocationsForAgent(agentID string) []ConfigLocation {
	var locations []ConfigLocation
	seen := map[string]bool{}
	add := func(label, path string) {
		if path == "" {
			return
		}
		abs, _ := filepath.Abs(path)
		norm := strings.ToLower(abs)
		if seen[norm] {
			return
		}
		seen[norm] = true
		exists := false
		accessible := false
		errStr := ""
		if _, err := os.Stat(path); err == nil {
			exists = true
			if _, err := os.ReadFile(path); err == nil {
				accessible = true
			} else {
				errStr = ShortText(err.Error(), 120)
			}
		}
		loc := ConfigLocation{Label: label, Path: path, Exists: exists, Accessible: accessible, Error: errStr}
		if exists && accessible {
			summary := ConfigProviderSummaryForAgent(path, agentID)
			loc.ProviderCount = len(summary.Providers)
			ids := make([]string, len(summary.Providers))
			for i, p := range summary.Providers {
				ids[i] = p.ID
			}
			loc.ProviderIDs = ids
		}
		locations = append(locations, loc)
	}
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	normAgent := NormalizeAgentID(agentID)
	if normAgent == AgentDeepSeek {
		add("DeepSeek 配置 (settings.yaml)", filepath.Join(DshHome(), "settings.yaml"))
		add("DeepSeek 配置 (settings.yml)", filepath.Join(DshHome(), "settings.yml"))
		add("DeepSeek 配置 (settings.json)", filepath.Join(DshHome(), "settings.json"))
	} else if normAgent == AgentOpenCode {
		add("OpenCode 配置", OpencodeConfig())
	} else if agentID == "" {
		add("ZCode 全局配置", ZCodeConfig())
		add("ZCode 旧版配置位置", filepath.Join(home, ".zcode", "config.json"))
		add("OpenCode 配置", OpencodeConfig())
		add("DeepSeek 配置 (settings.yaml)", filepath.Join(DshHome(), "settings.yaml"))
		add("DeepSeek 配置 (settings.json)", filepath.Join(DshHome(), "settings.json"))
	} else {
		add("ZCode 全局配置", ZCodeConfig())
		add("ZCode 旧版配置位置", filepath.Join(home, ".zcode", "config.json"))
	}
	return locations
}

func ConfigProviderSummary(path string) ImportPreview {
	return ConfigProviderSummaryForAgent(path, "")
}

func ConfigProviderSummaryForAgent(path string, agentID string) ImportPreview {
	item := ImportPreview{Path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return item
	}
	normAgent := ""
	if agentID != "" {
		normAgent = string(NormalizeAgentID(agentID))
	} else {
		low := strings.ToLower(path)
		if strings.Contains(low, ".dsh") {
			normAgent = string(AgentDeepSeek)
		} else if strings.Contains(low, "opencode") {
			normAgent = string(AgentOpenCode)
		}
	}
	if normAgent == string(AgentDeepSeek) {
		preview := DeepSeekImportPreview(path)
		preview.Path = path
		return preview
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		item.Error = err.Error()
		item.Exists = true
		return item
	}
	item.Exists = true
	item.Providers = BuildProviderSummary(cfg)
	return item
}

func deepCopyValue(v interface{}) interface{} {
	b, _ := json.Marshal(v)
	var out interface{}
	_ = json.Unmarshal(b, &out)
	return out
}
