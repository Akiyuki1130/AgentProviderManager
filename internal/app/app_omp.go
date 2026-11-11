package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"agentprovidermanager/internal/core"
	"agentprovidermanager/internal/ompprovider"
)

// 本文件集中放 OMP（omp / PI Agent）在 app 层的接线：
//   - 默认模型读写绑定 GetOMPModelRoles / SetOMPModelRole（计划 §5.2 / §5.3）；
//   - 探测与拉取的 options 解析（headers / kind / authHeader，计划 §4.5 / G5 / G9）；
//   - omp 路径判定、默认目标路径与 config.yml 的拒绝逻辑（计划 §4.3 / §4.4 / G7 / G18）；
//   - configFileAgent 的 §9.1 嗅探顺序与 ZCode 专属绑定的 omp guard。
//
// 这里不直接操作 ompprovider 的读写细节，统一走下面几个薄适配器：
// A（internal/ompprovider）是非 core 包，签名一旦微调只需改适配器，不扩散到业务分支。

// zcodeOnlyGuard 给 ZCode 专属绑定加 omp 卫士：omp 下这些能力没有意义，
// 返回 WRONG_AGENT 而不是让调用方误以为操作生效。
func zcodeOnlyGuard(agent string) map[string]interface{} {
	if core.NormalizeAgentID(agent) == core.AgentOMP {
		return map[string]interface{}{
			"success":    false,
			"error":      "当前 Agent 为 omp，该操作仅适用于 ZCode 配置",
			"error_code": "WRONG_AGENT",
		}
	}
	return nil
}

// isOMPAgent 判定当前 Agent 是否为 omp。
func isOMPAgent(agent string) bool {
	return core.NormalizeAgentID(agent) == core.AgentOMP
}

// ompConfigFileRejectMessage 是「把 config.yml 当成 provider 目标」时的统一中文文案（G7）。
const ompConfigFileRejectMessage = "config.yml 不是模型配置文件，请选择 models.yml 或 models.json"

// guardedTargetPath 判定某个路径能否作为当前 Agent 的 provider 目标。
// ChooseConfigFile / SetConfigPath 共用同一口径：omp 的 config.yml 只承载 modelRoles，
// 绝不能成为 targetConfigPath（G7）；其它 Agent 不受影响（ConfigFormatAt 只在 omp 下返回 omp-config）。
// 提成纯函数是为了能单测，不必真的弹系统文件对话框。
func guardedTargetPath(agent, path string) error {
	if core.ConfigFormatAt(path, agent) != core.FormatOMPConfig {
		return nil
	}
	return errors.New(ompConfigFileRejectMessage)
}

// ompModelsPath / ompModelsWritePath / ompConfigPath 是 ompprovider 路径判定的适配器。
// needsMigration 为 true 表示只存在旧版 models.json，APM 只能只读解析、绝不新建 .yml（G1）。
func ompModelsPath() (string, bool) { return ompprovider.ModelsPath("") }

func ompModelsWritePath() (string, bool) { return ompprovider.ModelsWritePath("") }

func ompConfigPath() (string, bool) { return ompprovider.ConfigPath("") }

// loadOMPModelsFile 读取并解析 models.yml；不做任何迁移或写盘。
func loadOMPModelsFile(path string) (*ompprovider.ModelsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ompprovider.LoadModels(data)
}

// loadOMPConfigFile 读取并解析 config.yml。
func loadOMPConfigFile(path string) (*ompprovider.ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ompprovider.LoadConfig(data)
}

// ompConfigFileBytes 把 ConfigFile 序列化回写盘字节。
func ompConfigFileBytes(cf *ompprovider.ConfigFile) ([]byte, error) {
	return cf.Bytes()
}

// ompDefaultTargetPath 是 omp 的默认 provider 目标文件（models.yml；只有 models.json 时
// 退回只读的 models.json，由界面提示先运行 omp 完成迁移）。
func ompDefaultTargetPath() string {
	return core.AgentDefaultPath(core.AgentOMP)
}

// preferOMPModelsPath 只把「旧版 models.json」升级成可写的 models.yml：
// 用户在 SetConfigPath 里显式选的其它路径一律保持不动，避免启动时被悄悄改写。
func preferOMPModelsPath(stored string) string {
	if strings.TrimSpace(stored) == "" {
		return stored
	}
	if !strings.EqualFold(filepath.Base(stored), "models.json") {
		return stored
	}
	if !underOMPDirs(stored) {
		return stored
	}
	if writePath, writable := ompModelsWritePath(); writable && strings.TrimSpace(writePath) != "" {
		return writePath
	}
	return stored
}

// underOMPDirs 判定路径是否落在 omp 的配置目录里：OMP home、PI_CONFIG_DIR、
// PI_CODING_AGENT_DIR 以及命名 profile 的 agent 目录。IsUnderHome 已覆盖绝大多数
// 情况，这里额外兜一层 PI_CODING_AGENT_DIR，保证被环境变量指到别处的 agent 目录
// 也能被 §9.1 的路径分支识别。
func underOMPDirs(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if ompprovider.IsUnderHome(path) {
		return true
	}
	dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	if dir == "" {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ompLegacySettingsExist 判定 G18：config.yml / config.yaml 都不存在，但同目录的
// settings.json 还在——这是旧版 omp 尚未迁移的状态，APM 绝不能新建 config.yml
// （否则 omp 会永久跳过迁移）。
func ompLegacySettingsExist(configPath string) bool {
	if strings.TrimSpace(configPath) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(filepath.Dir(configPath), "settings.json"))
	return err == nil
}

// fetchOptionsFromMap 把绑定层传入的 opts map 解析成 core.FetchOptions，并给出
// 可喂给 BuildModelCardsForSite 的 apiType。opts 允许为 nil：此时行为与旧的两参
// 版本完全一致（无 headers、不指定 kind、apiType 空串 fail-closed）。
//
// opts 键（契约 §3.3）：headers(map[string]string)、kind(string)、auth_header(bool)。
func fetchOptionsFromMap(opts map[string]interface{}) (core.FetchOptions, string) {
	var opt core.FetchOptions
	apiType := ""
	if opts == nil {
		return opt, apiType
	}
	if rawHeaders, ok := opts["headers"]; ok && rawHeaders != nil {
		headers := map[string]string{}
		switch h := rawHeaders.(type) {
		case map[string]interface{}:
			for k, v := range h {
				if s, ok := v.(string); ok {
					headers[k] = s
				}
			}
		case map[string]string:
			for k, v := range h {
				headers[k] = v
			}
		}
		if len(headers) > 0 {
			opt.Headers = headers
		}
	}
	if kind, ok := opts["kind"].(string); ok {
		kind = strings.TrimSpace(kind)
		if kind != "" {
			opt.Kind = kind
			// 只有三种主流协议参与站点规则匹配；其它值保持 fail-closed。
			if norm, err := core.ValidateKind(kind); err == nil {
				apiType = norm
			}
		}
	}
	if v, ok := opts["auth_header"].(bool); ok {
		authHeader := v
		opt.AuthHeader = &authHeader
	}
	return opt, apiType
}

// providerRequestOptions 从 ProviderEdit 反推探测/拉取所需的 opts（契约 §3.3）。
// kind 对任何 agent 都透传（站点规则匹配需要协议类型，G16）；headers / authHeader
// 只对 omp 透传——它们是 omp 的 options_json 语义，别的 harness 的同名字段含义不同。
func providerRequestOptions(edit *core.ProviderEdit, agent string) map[string]interface{} {
	if edit == nil {
		return nil
	}
	opts := map[string]interface{}{}
	if kind := strings.TrimSpace(edit.Kind); kind != "" {
		opts["kind"] = kind
	}
	if isOMPAgent(agent) && strings.TrimSpace(edit.OptionsJSON) != "" {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(edit.OptionsJSON), &raw); err == nil && raw != nil {
			if headers, ok := raw["headers"].(map[string]interface{}); ok && len(headers) > 0 {
				opts["headers"] = headers
			}
			if authHeader, ok := raw["authHeader"].(bool); ok {
				opts["auth_header"] = authHeader
			}
		}
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

// ompModelSelectors 扫描 models.yml 生成默认模型卡片的选择器列表。
// 只有显式声明模型、且 api 属于三种主流协议的 provider 才会出现在里面（由 core 决定）。
//
// 文件不存在不是错误：omp 首次运行前 ~/.omp/agent 里可能一个模型文件都没有，
// 此时应显示「还没有 models.yml」的空状态，保存第一个提供商时再由写盘路径创建。
func ompModelSelectors() ([]string, bool, error) {
	modelsPath, needsMigration := ompModelsPath()
	if needsMigration || strings.TrimSpace(modelsPath) == "" {
		return []string{}, needsMigration, nil
	}
	models, err := loadOMPModelsFile(modelsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, false, nil
		}
		return []string{}, needsMigration, err
	}
	return core.OMPSelectors(models), needsMigration, nil
}

// ompModelRoles 读取 config.yml 里的 modelRoles；文件不存在时返回空映射而不是错误。
func ompModelRoles(configPath string, exists bool) (map[string]string, error) {
	roles := map[string]string{}
	if !exists || strings.TrimSpace(configPath) == "" {
		return roles, nil
	}
	cf, err := loadOMPConfigFile(configPath)
	if err != nil {
		return roles, err
	}
	parsed, err := cf.ModelRoles()
	if err != nil {
		return roles, err
	}
	if parsed != nil {
		roles = parsed
	}
	return roles, nil
}

// GetOMPModelRoles 返回 omp 当前的默认模型绑定与可选选择器（计划 §5.2）。
// selectors 由后端扫描 models.yml 生成，与当前 default 无关。
func (a *App) GetOMPModelRoles() map[string]interface{} {
	if !isOMPAgent(a.currentAgent) {
		return map[string]interface{}{
			"success":    false,
			"error":      "当前 Agent 不是 omp，无法读取默认模型",
			"error_code": "WRONG_AGENT",
		}
	}
	configPath, configExists := ompConfigPath()
	modelsPath, _ := ompModelsPath()
	modelsExists := false
	if strings.TrimSpace(modelsPath) != "" {
		if _, statErr := os.Stat(modelsPath); statErr == nil {
			modelsExists = true
		}
	}
	resp := map[string]interface{}{
		"success":       true,
		"path":          configPath,
		"exists":        configExists,
		"models_path":   modelsPath,
		"models_exists": modelsExists,
		"roles":         map[string]string{},
		"selectors":     []string{},
	}
	selectors, needsMigration, selErr := ompModelSelectors()
	resp["needs_migration"] = needsMigration
	if selErr != nil {
		return map[string]interface{}{
			"success":    false,
			"path":       configPath,
			"exists":     configExists,
			"error":      core.ShortText("读取 omp models.yml 失败："+selErr.Error(), 300),
			"error_code": "READ_FAILED",
		}
	}
	resp["selectors"] = selectors
	roles, rerr := ompModelRoles(configPath, configExists)
	if rerr != nil {
		return map[string]interface{}{
			"success":    false,
			"path":       configPath,
			"exists":     configExists,
			"error":      core.ShortText("读取 omp config.yml 失败："+rerr.Error(), 300),
			"error_code": "READ_FAILED",
		}
	}
	resp["roles"] = roles
	return resp
}

// SetOMPModelRole 设置 omp 的默认模型（P1 只允许 role=default，见计划 §5.2 / §5.3）。
//
// 写入顺序严格按 §5.3：Agent 校验 -> role 白名单 -> 精确选择器校验（空串 = 删除键）
// -> 读 config.yml 校验根 mapping -> G18 旧版迁移保护 -> G7 拒绝把 config.yml 当 provider
// 目标 -> PrepareChange（format=omp-config, op=set_default_model）-> WriteConfigBytes。
func (a *App) SetOMPModelRole(role, selector string) map[string]interface{} {
	role = strings.TrimSpace(role)
	selector = strings.TrimSpace(selector)
	a.mu.Lock()
	defer a.mu.Unlock()
	if !isOMPAgent(a.currentAgent) {
		return map[string]interface{}{
			"success":    false,
			"error":      "当前 Agent 不是 omp，无法设置默认模型",
			"error_code": "WRONG_AGENT",
		}
	}
	if role != "default" {
		return map[string]interface{}{
			"success":    false,
			"error":      "当前仅支持设置默认模型（role=default）",
			"error_code": "BAD_ROLE",
		}
	}
	// P1 只接受精确选择器："provider/model"。glob 与 @role 别名留给后续版本，
	// 这里提前拒绝，避免把 omp 解析不了的值写进 config.yml。
	if strings.ContainsAny(selector, "*?") {
		return map[string]interface{}{
			"success":    false,
			"error":      "默认模型只支持精确选择器（provider/model），不支持通配符",
			"error_code": "BAD_SELECTOR",
		}
	}
	if strings.HasPrefix(selector, "@") {
		return map[string]interface{}{
			"success":    false,
			"error":      "默认模型只支持精确选择器（provider/model），不支持 @ 角色引用",
			"error_code": "BAD_SELECTOR",
		}
	}
	configPath, configExists := ompConfigPath()
	if strings.TrimSpace(configPath) == "" {
		return map[string]interface{}{"success": false, "error": "未找到 omp 配置文件路径", "error_code": "NO_CONFIG"}
	}
	// G7 双保险：config.yml 只能作为默认模型的读写目标，绝不能是当前 provider 目标。
	if stored := strings.TrimSpace(a.agentPaths[string(core.AgentOMP)]); stored != "" &&
		strings.EqualFold(filepath.Clean(stored), filepath.Clean(configPath)) {
		return map[string]interface{}{
			"success":    false,
			"error":      "omp 的 provider 目标不能是 config.yml，请重新选择 models.yml",
			"error_code": "BAD_TARGET",
		}
	}
	// 选择器必须真实存在于 models.yml，避免写进一个 omp 解析不到的默认模型。
	if selector != "" {
		selectors, needsMigration, selErr := ompModelSelectors()
		if selErr != nil {
			return map[string]interface{}{
				"success":    false,
				"error":      core.ShortText("读取 omp models.yml 失败："+selErr.Error(), 300),
				"error_code": "READ_FAILED",
			}
		}
		if needsMigration {
			return map[string]interface{}{
				"success":    false,
				"error":      "检测到旧版 models.json，请先运行一次 omp 完成迁移",
				"error_code": "NEEDS_MIGRATION",
			}
		}
		found := false
		for _, s := range selectors {
			if s == selector {
				found = true
				break
			}
		}
		if !found {
			return map[string]interface{}{
				"success":    false,
				"error":      "选择器「" + selector + "」不在 models.yml 中，请先在提供商里保存该模型",
				"error_code": "BAD_SELECTOR",
			}
		}
	}
	if !configExists {
		// G18：旧版 settings.json 还在时绝不新建 config.yml，否则 omp 会永久跳过迁移。
		if ompLegacySettingsExist(configPath) {
			return map[string]interface{}{
				"success":    false,
				"error":      "omp 尚未完成旧版设置迁移，请先运行一次 omp 完成迁移后再设置默认模型",
				"error_code": "NEEDS_MIGRATION",
			}
		}
		return map[string]interface{}{
			"success":    false,
			"error":      "未找到 omp 的 config.yml，请先运行一次 omp 生成配置",
			"error_code": "NO_CONFIG",
		}
	}
	// 校验根 mapping：config.yml 必须是 YAML 映射（空文件 / 纯注释视为空文档，
	// 由 ConfigFile 在写入时补出 modelRoles 映射），否则写回会破坏 omp 配置。
	if data, err := os.ReadFile(configPath); err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText("读取 omp config.yml 失败："+err.Error(), 300), "error_code": "READ_FAILED"}
	} else {
		var root map[string]interface{}
		if yerr := yaml.Unmarshal(data, &root); yerr != nil {
			return map[string]interface{}{"success": false, "error": "omp config.yml 的根节点不是映射，已取消写入", "error_code": "BAD_CONFIG"}
		}
	}
	cf, err := loadOMPConfigFile(configPath)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText("解析 omp config.yml 失败："+err.Error(), 300), "error_code": "BAD_CONFIG"}
	}
	// 空 selector 表示删除默认模型绑定：SetModelRole 内部按“写空即删除键”处理，
	// 绝不写 modelRoles.default: ""（omp 会把它当成无效选择器）。
	if err := cf.SetModelRole(role, selector); err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300), "error_code": "BAD_SELECTOR"}
	}
	out, err := ompConfigFileBytes(cf)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText("序列化 omp config.yml 失败："+err.Error(), 300), "error_code": "OTHER"}
	}
	fingerprint, _ := core.FileFingerprint(configPath)
	snapshot, err := core.PrepareChange(core.ChangeContext{
		Target:    configPath,
		AgentID:   string(core.AgentOMP),
		Format:    core.FormatOMPConfig,
		Operation: core.OpSetDefaultModel,
		Note:      "设置 omp 默认模型",
	})
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300), "error_code": "OTHER"}
	}
	if err := core.WriteConfigBytes(configPath, out, fingerprint); err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText("写入 omp config.yml 时出错："+err.Error(), 300), "error_code": "OTHER"}
	}
	roles, _ := cf.ModelRoles()
	if roles == nil {
		roles = map[string]string{}
	}
	return map[string]interface{}{
		"success":       true,
		"path":          configPath,
		"roles":         roles,
		"backup":        snapshot.BackupPath,
		"restore_point": snapshot.RestorePoint,
	}
}

// configFileAgent 判定一份配置文件属于哪个 Agent，并返回其格式名（计划 §9.1）。
// 只按路径与内容判定，供“选择一份配置文件合并进当前 Agent”的入口使用。
func configFileAgent(path string) (string, string) {
	agent, format, _ := configFileAgentDetail(path)
	return agent, format
}

// configFileAgentDetail 是 configFileAgent 的完整版本，额外给出「为什么不能当
// provider 文件用」的中文原因（目前只有 omp 的 config.yml 会命中）。
func configFileAgentDetail(path string) (string, string, string) {
	if strings.TrimSpace(path) == "" {
		return "", "", ""
	}
	// ① 路径优先：omp 配置目录里的文件名本身就是最强的身份信号。
	// 注意 IsConfigFileName 对 models.* 与 config.* 都返回 true，所以必须先判 models。
	if underOMPDirs(path) {
		switch strings.ToLower(filepath.Base(path)) {
		case "models.yml", "models.yaml", "models.json":
			return string(core.AgentOMP), "omp-models", ""
		}
		if ompprovider.IsConfigFileName(path) {
			return "", "", "这是 omp 的设置文件（config.yml），不是提供商配置文件；默认模型请使用默认模型卡片修改"
		}
	}
	// ② 内容嗅探：YAML 根 mapping 且含 providers → omp（omp 的 models.yml 结构）。
	if sniffOMPModelsContent(path) {
		return string(core.AgentOMP), "omp-models", ""
	}
	// DeepSeek Harness 与 ZCode 继续沿用 core 的格式探测器（顺序与旧实现一致）。
	if format := core.DeepSeekFormatName(core.DetectDeepSeekFormatAt(path)); format != "unknown" {
		return string(core.AgentDeepSeek), format, ""
	}
	if format := core.AgentFormatName(string(core.AgentZCode), path); format != "unknown" {
		return string(core.AgentZCode), format, ""
	}
	return "", "", ""
}

// sniffOMPModelsContent 按 §9.1 ② 嗅探 omp 的 models.yml：YAML 根节点是 mapping
// 且含 providers 键。JSON 不走这里（JSON 顶层 provider 属于 ZCode legacy 的形状）。
func sniffOMPModelsContent(path string) bool {
	low := strings.ToLower(path)
	if !strings.HasSuffix(low, ".yml") && !strings.HasSuffix(low, ".yaml") {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil || root == nil {
		return false
	}
	// 只认“根 mapping 且声明了 providers 键”，与 ompprovider.Detect 的口径一致；
	// providers 为空映射也仍算 omp 模型文件（此时没有任何可选模型）。
	providers, ok := root["providers"]
	if !ok {
		return false
	}
	switch providers.(type) {
	case nil, map[string]interface{}:
		return true
	}
	return false
}
