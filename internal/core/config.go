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

	"agentprovidermanager/internal/dshprovider"
	"agentprovidermanager/internal/ompprovider"
	"agentprovidermanager/internal/zcodeprovider"

	"gopkg.in/yaml.v3"
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

// LoadConfigForRestore 按备份路径的格式选择解析器。isYAMLPath 会先剥离 .bak_<ts> /
// .snapshot_<ts> 等固定词表后缀，所以 settings.yaml.bak_<ts> 与 settings.yaml 走同一条
// YAML 分支；.json 系列备份也仍按 JSON 解析。恢复入口（RestoreBackup / RestoreLastBackup）
// 与还原点校验（validateRestoreContent）共用这一处判定。
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
	text, _ := json.MarshalIndent(config, "", "  ")
	text = append(text, '\n')
	return WriteConfigBytes(path, text, expectedFingerprint)
}

// guardZCodeV2Doc 阻止把 ZCode 不接受的文档写进新版个人配置。
//
// ZCode 的 zod schema 是 .strict()：任何未知键都会让整份个人配置被当成空，所有供应商
// 在使用端消失。新版文档必须整体经过 zcodeprovider 的严格校验才能落盘；而旧式的
// “LoadConfig -> 改顶层 provider map -> WriteConfig” 写法会往新版文档里塞一个顶层
// provider 键，正是这条规则要拦住的。所有写盘最终都经过 WriteConfigBytes，因此闸门
// 放在这里，任何新增的调用方都不会绕过它。
//
// 只识别“顶层同时有 schemaVersion 与 config”的 JSON：这是 ZCode 新版个人配置的特征，
// opencode / deepseek 的配置不会命中，不会被误伤。
func guardZCodeV2Doc(data []byte) error {
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	if _, ok := doc["schemaVersion"]; !ok {
		return nil
	}
	if _, ok := doc["config"]; !ok {
		return nil
	}
	if _, err := zcodeprovider.Decode(data); err != nil {
		return &ConfigParseError{Msg: fmt.Sprintf(
			"拒绝写入：这份 JSON 是 ZCode 新版个人配置（顶层有 schemaVersion 与 config），但没有通过 ZCode 的严格校验（%v）。ZCode 遇到未知键会把整份个人配置当成空，因此本次写入已取消。",
			err)}
	}
	return nil
}

// WriteConfigBytes 是 WriteConfig 的字节写入部分：同样的目录创建、原子替换
// （临时文件 + rename）与指纹校验，但直接写出给定字节。
// 新版 ZCode provider_config.json 走 zcodeprovider.Encode 得到 canonical 字节，
// 若这里再走 map，就会丢掉严格校验（未知键会让 ZCode 把整份配置当成空）。
func WriteConfigBytes(path string, data []byte, expectedFingerprint string) error {
	if err := guardZCodeV2Doc(data); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	d := dir
	if d == "" {
		d = "."
	}
	tmpFile, err := os.CreateTemp(d, filepath.Base(path)+".tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
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
	// needsMigration 只对 omp 的 models.json 位置为 true（G1）；其它 harness 恒为 false。
	add := func(label, path string, needsMigration ...bool) {
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
		loc.NeedsMigration = exists && len(needsMigration) > 0 && needsMigration[0]
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
	if normAgent == AgentOMP {
		// 计划 §4.4：只列实际存在的 models 文件；config.yml 永不出现在这里（G7）。
		ompAddLocations(add)
		if len(locations) == 0 {
			// omp 尚未产生任何 models 文件时，仍给一个「尚不存在但可写」的候选，
			// 与其它 agent 列出默认路径的口径一致；ModelsWritePath 在只有 models.json
			// 时返回不可写（G1），因此这里不会诱导新建 .yml 去遮蔽 .json。
			if path, writable := ompprovider.ModelsWritePath(""); writable {
				add(ompDefaultModelsLabel(ompprovider.AgentDir("")), path)
			}
		}
	} else if normAgent == AgentDeepSeek {
		// 0.2：桌面端与 web profile 各自一个 patch；home 级 patch 优先级更高。
		add("DeepSeek Harness 桌面端 profile（0.2）", DeepSeekDesktopProfilePatch())
		add("DeepSeek Harness web profile（0.2）", DeepSeekWebProfilePatch())
		for _, path := range DeepSeekProfilePatches() {
			if name := dshprovider.ProfileNameAt(path); name != "" && name != dshprovider.DesktopProfileName && name != dshprovider.WebProfileName {
				add(fmt.Sprintf("DeepSeek Harness profile「%s」（0.2）", name), path)
			}
		}
		add("DeepSeek Harness home 级 patch（0.2，优先级最高）", DeepSeekHomePatch())
		// 0.1 的 settings.* 只作为旧格式保留（0.2 只会导入一次）。
		add("DeepSeek 旧版配置 (settings.yaml)", filepath.Join(DshHome(), "settings.yaml"))
		add("DeepSeek 旧版配置 (settings.yml)", filepath.Join(DshHome(), "settings.yml"))
		add("DeepSeek 旧版配置 (settings.json)", filepath.Join(DshHome(), "settings.json"))
	} else if normAgent == AgentOpenCode {
		add("OpenCode 配置", OpencodeConfig())
	} else if agentID == "" {
		before := len(locations)
		// 计划 §9.1：总览同样要能看到 omp；这里只报实际存在的 models 文件。
		ompAddLocations(add)
		if len(locations) == before {
			// 一个 models 文件都没有时，仍给一个可写候选（与 omp 单 agent 分支同口径）。
			if path, writable := ompprovider.ModelsWritePath(""); writable {
				add(ompDefaultModelsLabel(ompprovider.AgentDir("")), path)
			}
		}
		add("ZCode 全局配置", ZCodeConfig())
		add("ZCode 新版配置", ZCodeProviderConfigPath())
		add("ZCode 旧版配置位置", filepath.Join(home, ".zcode", "config.json"))
		add("OpenCode 配置", OpencodeConfig())
		add("DeepSeek 配置 (settings.yaml)", filepath.Join(DshHome(), "settings.yaml"))
		add("DeepSeek 配置 (settings.json)", filepath.Join(DshHome(), "settings.json"))
	} else {
		add("ZCode 全局配置", ZCodeConfig())
		add("ZCode 新版配置", ZCodeProviderConfigPath())
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
		} else {
			// 计划 §9.1：总览（agentID 为空）同样按内容嗅探，否则 omp 的 models.yml
			// 会被当成 ZCode JSON 解析而报错、显示 0 个 provider。
			normAgent = sniffConfigAgent(path)
		}
	}
	if AgentID(normAgent) == AgentOMP {
		if IsOMPConfigFile(path) {
			// G7：config.yml 不是 provider 文件，绝不能按 providers 解析。
			return ImportPreview{Path: path, Exists: true, Error: "这是 omp 的设置文件（modelRoles 等），不是 provider 配置；请选择 models.yml"}
		}
		return ompProviderSummary(path)
	}
	if normAgent == "omp-config" {
		return ImportPreview{Path: path, Exists: true, Error: "这是 omp 的设置文件（modelRoles 等），不是 provider 配置；请选择 models.yml"}
	}
	if normAgent == string(AgentDeepSeek) {
		preview := DeepSeekImportPreview(path)
		preview.Path = path
		return preview
	}
	// 新版 provider_config.json 没有 legacy 的 provider 键，供应商在
	// providerConfigRules.providerRules 里；按内容分流，否则该条目会显示 0 个供应商。
	// 其它 agent 的配置（OpenCode 等）即便顶层带 config/schemaVersion 也不走这条路。
	if normAgent != string(AgentOpenCode) && DetectZCodeFormatAt(path) == zcodeprovider.FormatV2 {
		return v2ProviderImportPreview(path, item)
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

// v2ProviderImportPreview 严格解码新版 provider_config.json，并按 providerRule 生成摘要。
// 解码失败时沿用 ImportPreview.Error（不新增字段）。
func v2ProviderImportPreview(path string, item ImportPreview) ImportPreview {
	item.Exists = true
	data, err := os.ReadFile(path)
	if err != nil {
		item.Error = fmt.Sprintf("读取配置文件失败：%v", err)
		return item
	}
	cfg, err := zcodeprovider.Decode(data)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	item.Providers = zcodeV2Store{}.List(cfg.Doc())
	return item
}

// ompAddLocations 按计划 §4.4 的顺序枚举 omp 的 models 文件位置：
// 默认 profile、命名 profile（按 profile 名排序）。
// 每一项都只报告「实际存在的文件」，needsMigration 表示当前只有 models.json。
func ompAddLocations(add func(label, path string, needsMigration ...bool)) {
	if ompprovider.Home() == "" {
		return
	}
	// 默认 profile 的目录必须与 omp 的读取语义同源：设置了 PI_CODING_AGENT_DIR 时
	// omp 根本不读 <home>/agent，把它列成「默认 profile」会让用户去编辑一个死文件。
	defaultDir := ompprovider.AgentDir("")
	if path, exists, migration := ompModelsFileInDir(defaultDir); exists {
		add(ompDefaultModelsLabel(defaultDir), path, migration)
	}
	for _, name := range ompprovider.Profiles() {
		if path, exists, migration := ompModelsFileInDir(ompprovider.AgentDir(name)); exists {
			add(fmt.Sprintf("PI Agent profile「%s」", name), path, migration)
		}
	}
}

// ompDefaultModelsLabel 说明默认 profile 位置的来源：PI_CODING_AGENT_DIR 生效时给出
// 环境变量提示，避免用户以为自己在改 <home>/agent。
func ompDefaultModelsLabel(defaultDir string) string {
	override := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	if override != "" && strings.EqualFold(filepath.Clean(override), filepath.Clean(defaultDir)) {
		return "PI Agent 配置（PI_CODING_AGENT_DIR）"
	}
	return "PI Agent 配置（默认 profile）"
}

// ompModelsFileInDir 在指定目录里按 omp 的读取优先级找 models 文件：
// .yml > .yaml > .json；只有 .json 时 exists=true 且 needsMigration=true（G1）。
func ompModelsFileInDir(dir string) (path string, exists bool, needsMigration bool) {
	if strings.TrimSpace(dir) == "" {
		return "", false, false
	}
	for _, name := range []string{"models.yml", "models.yaml"} {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true, false
		}
	}
	p := filepath.Join(dir, "models.json")
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return p, true, true
	}
	return filepath.Join(dir, "models.yml"), false, false
}

// ompProviderSummary 用 ompprovider 的读侧 + omp_models.go 的只读判定生成位置列表摘要，
// 保证列表里的 kind / 只读徽标与编辑器（OMPProviderView）完全同源。
// 只有 models.json 时整份文件都是只读可见（G1）。
func ompProviderSummary(path string) ImportPreview {
	item := ImportPreview{Path: path, Exists: true}
	data, err := os.ReadFile(path)
	if err != nil {
		item.Error = ShortText(fmt.Sprintf("读取配置文件失败：%v", err), 200)
		return item
	}
	migrationOnly := strings.EqualFold(filepath.Ext(path), ".json")
	models, err := ompprovider.LoadModels(data)
	if err != nil {
		item.Error = ShortText(err.Error(), 200)
		return item
	}
	providers, err := models.Providers()
	if err != nil {
		item.Error = ShortText(err.Error(), 200)
		return item
	}
	ids := models.ProviderIDs()
	sort.Strings(ids)
	for _, id := range ids {
		view := ompReadProvider(id, providers[id], "")
		item.Providers = append(item.Providers, ProviderSummary{
			ID:         id,
			Name:       id,
			Kind:       view.Kind,
			BaseURL:    view.BaseURL,
			HasAPIKey:  view.APIKeyRaw != "",
			ModelCount: len(view.ModelIDs),
			ReadOnly:   view.ReadOnly || migrationOnly,
		})
	}
	return item
}

// IsOMPConfigFile 判定路径是否是 omp 的设置文件（config.yml / config.yaml）。
// G7：它永远不能作为 provider 目标，app 层的 SetConfigPath / ChooseConfigFile 必须拒绝；
// 判定同时要求文件确实位于 omp 目录（OMPHome / PI_CODING_AGENT_DIR），避免误伤其它
// harness 里恰好叫 config.yml 的文件。
func IsOMPConfigFile(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	switch strings.ToLower(filepath.Base(stripAuxSuffix(path))) {
	case "config.yml", "config.yaml":
	default:
		return false
	}
	return ompPathOwned(path)
}

// ompPathOwned 判断路径是否位于 omp 的配置目录内。
func ompPathOwned(path string) bool {
	if ompprovider.IsUnderHome(path) {
		return true
	}
	override := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	if override == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	dir, err := filepath.Abs(override)
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Dir(abs), dir)
}

// sniffConfigAgent 按计划 §9.1 的内容顺序判定配置文件属于哪个 harness。
// 返回 "" 表示不命中，调用方继续走既有的 ZCode 格式判定。
func sniffConfigAgent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if strings.Contains(strings.ToLower(string(data)), "llm-pi-ai") {
		return string(AgentDeepSeek)
	}
	owned := ompPathOwned(path)
	if owned && IsOMPConfigFile(path) {
		return "omp-config"
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil || root == nil {
		return ""
	}
	// omp models 的根是 mapping 且带 providers 键（ZCode legacy 用单数 provider，
	// ZCode v2 用 schemaVersion + config，都不会命中这里）。
	if _, ok := root["providers"]; ok {
		return string(AgentOMP)
	}
	if owned && root["modelRoles"] != nil {
		return "omp-config"
	}
	return ""
}

func deepCopyValue(v interface{}) interface{} {
	b, _ := json.Marshal(v)
	var out interface{}
	_ = json.Unmarshal(b, &out)
	return out
}
