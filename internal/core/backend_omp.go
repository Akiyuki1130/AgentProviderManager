package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentprovidermanager/internal/ompprovider"
	"gopkg.in/yaml.v3"
)

// omp（oh-my-pi）的 backend 注册。
//
// omp 的 provider 只存在于 models.yml / models.yaml（models.json 只能只读解析，
// 等 omp 自己迁移，G1）。config.yml 是模型角色与 preset 的所在地，不是 provider
// 目标，因此这里只注册一个默认 backend、不注册格式探测：GetBackend 会直接回退到它。
func init() {
	RegisterBackendFor(string(AgentOMP), ompBackend{})
	// 把 internal/ompprovider 的真实解析器注入 store 的读取入口。
	// *ompprovider.ModelsFile 同时满足 OMPModelsRead 与 OMPModelsWrite。
	ompLoadModelsFile = func(data []byte) (OMPModelsRead, error) {
		file, err := ompprovider.LoadModels(data)
		if err != nil {
			return nil, err
		}
		if file == nil {
			return nil, fmt.Errorf("omp 模型文件解析结果为空")
		}
		return file, nil
	}
}

type ompBackend struct{}

func (ompBackend) Descriptor() BackendDescriptor {
	return BackendDescriptor{
		ID:          string(AgentOMP),
		Label:       "OMP",
		DefaultPath: AgentDefaultPath(AgentOMP),
		// Candidates 必须保持空切片：omp 的候选位置由 DetectConfigLocationsForAgent
		// 统一探测（C 负责），backend 自己再探测会互相递归。
		Candidates: []ConfigLocation{},
	}
}

func (ompBackend) Read(path string) (Doc, error) {
	doc, _, err := ompReadDoc(path)
	return doc, err
}

func (ompBackend) ReadWithFingerprint(path string) (Doc, string, error) {
	before, _ := FileFingerprint(path)
	doc, _, err := ompReadDoc(path)
	if err != nil {
		return nil, "", err
	}
	after, _ := FileFingerprint(path)
	if before != after {
		return nil, "", fmt.Errorf("配置文件在读取期间被其他程序修改，请重试")
	}
	return doc, after, nil
}

// Write 把内存里的 AST 序列化后写盘。
//
// 唯一的写盘出口是 WriteConfigBytes（原子临时文件 + Rename + 指纹校验）；
// 这里额外做三件事：拒绝 models.json（G1）、写前 Validate()、把 "_" 前缀内部键
// 挡在序列化之外（内部键只存在于 Doc，不进 AST，所以 Bytes() 天然干净）。
func (ompBackend) Write(path string, doc Doc, expectedFingerprint string) error {
	if !ompDocBoolValue(doc, ompDocWritable) {
		return fmt.Errorf("当前 omp 模型文件只能只读解析（只有 models.json），请先运行一次 omp 完成 models.json 迁移")
	}
	file, err := ompDocModels(doc)
	if err != nil {
		return err
	}
	writer, ok := file.(OMPModelsWrite)
	if !ok {
		return ompWriteNotReadyError()
	}
	if err := writer.Validate(); err != nil {
		return err
	}
	data, err := file.Bytes()
	if err != nil {
		return fmt.Errorf("序列化 omp 模型文件失败：%v", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return fmt.Errorf("omp 模型文件内容为空，已取消写入")
	}
	return WriteConfigBytes(path, data, expectedFingerprint)
}

func (ompBackend) Store(Doc) ProviderStore { return ompStore{} }

// ompReadDoc 读取并解析 omp 模型文件，产出带内部键的 Doc。
func ompReadDoc(path string) (Doc, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "", fmt.Errorf("未指定 omp 模型文件路径")
	}
	fileName := filepath.Base(path)
	needsMigration := strings.EqualFold(filepath.Ext(path), ".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, "", fmt.Errorf("读取 omp 模型文件失败：%v", err)
		}
		// omp 还没生成 models.yml：给一个空文档，让页面能直接新建 provider。
		file := ompprovider.NewModelsFile()
		roles, presets, cfgErr := ompReadConfigFile(path)
		doc := Doc{
			ompDocFile:           file,
			ompDocPath:           path,
			ompDocFileName:       fileName,
			ompDocWritable:       true,
			ompDocNeedsMigration: false,
			ompDocAlias:          map[string]string{},
			ompDocRoles:          roles,
			ompDocPresets:        presets,
			ompDocRefs:           ompBuildReferences(nil, roles, presets),
		}
		if cfgErr != nil {
			doc[ompDocConfigErr] = cfgErr
		}
		return doc, "", nil
	}

	format := ompprovider.Detect(path, data)
	switch format {
	case ompprovider.FormatModelsYAML, ompprovider.FormatModelsJSON:
	case ompprovider.FormatConfigYAML:
		return nil, "", fmt.Errorf("config.yml 不是 omp 的提供商配置；omp 的 provider 只存在于 models.yml")
	default:
		return nil, "", fmt.Errorf("该文件不是 omp 的模型文件（期望 models.yml / models.yaml / models.json）")
	}

	file, err := ompLoadModelsFile(data)
	if err != nil {
		return nil, "", fmt.Errorf("解析 omp 模型文件失败：%v", err)
	}
	providers, err := file.Providers()
	if err != nil {
		return nil, "", fmt.Errorf("解析 omp 提供商失败：%v", err)
	}
	roles, presets, cfgErr := ompReadConfigFile(path)
	doc := Doc{
		ompDocFile:           file,
		ompDocPath:           path,
		ompDocFileName:       fileName,
		ompDocWritable:       !needsMigration,
		ompDocNeedsMigration: needsMigration,
		ompDocAlias:          ompScanAliasReasons(data),
		ompDocRoles:          roles,
		ompDocPresets:        presets,
		ompDocRefs:           ompBuildReferences(providers, roles, presets),
	}
	if cfgErr != nil {
		doc[ompDocConfigErr] = cfgErr
	}
	return doc, "", nil
}

// ompReadConfigFile 只读扫描 models 文件同目录下的 config.yml / config.yaml，
// 取 modelRoles 与 modelPresets 供引用保护使用。P1 不改这两个键（除了
// SetOMPModelRole 写 modelRoles.default，由 app 层单独走 PrepareChange）。
func ompReadConfigFile(modelsPath string) (map[string]string, map[string]OMPPreset, error) {
	roles := map[string]string{}
	presets := map[string]OMPPreset{}
	dir := filepath.Dir(modelsPath)
	path := filepath.Join(dir, "config.yml")
	if _, err := os.Stat(path); err != nil {
		alternate := filepath.Join(dir, "config.yaml")
		if _, err2 := os.Stat(alternate); err2 != nil {
			return roles, presets, nil
		}
		path = alternate
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return roles, presets, fmt.Errorf("读取 omp config.yml 失败：%v", err)
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return roles, presets, fmt.Errorf("解析 omp config.yml 失败：%v", err)
	}
	for role, value := range ompMap(root["modelRoles"]) {
		if selector, ok := value.(string); ok && strings.TrimSpace(selector) != "" {
			roles[role] = strings.TrimSpace(selector)
		}
	}
	for name, value := range ompMap(root["modelPresets"]) {
		preset := OMPPreset{ModelRoles: map[string]string{}}
		entry := ompMap(value)
		for role, selector := range ompMap(entry["modelRoles"]) {
			if s, ok := selector.(string); ok && strings.TrimSpace(s) != "" {
				preset.ModelRoles[role] = strings.TrimSpace(s)
			}
		}
		if level, ok := entry["defaultThinkingLevel"].(string); ok {
			preset.DefaultThinkingLevel = strings.TrimSpace(level)
		}
		presets[name] = preset
	}
	return roles, presets, nil
}

// ompBuildReferences 从 providers 与 config.yml 汇总引用保护所需信息。
func ompBuildReferences(providers map[string]map[string]interface{}, roles map[string]string, presets map[string]OMPPreset) OMPReferences {
	refs := OMPReferences{
		Roles:   roles,
		Presets: map[string]map[string]string{},
		InFile:  map[string][]string{},
		Models:  map[string][]string{},
	}
	if refs.Roles == nil {
		refs.Roles = map[string]string{}
	}
	for name, preset := range presets {
		if len(preset.ModelRoles) == 0 {
			continue
		}
		copied := make(map[string]string, len(preset.ModelRoles))
		for role, selector := range preset.ModelRoles {
			copied[role] = selector
		}
		refs.Presets[name] = copied
	}
	for providerID, raw := range providers {
		ids := make([]string, 0, len(raw))
		for _, model := range ompModelList(raw) {
			modelID := strings.TrimSpace(stringOr(model["id"], ""))
			if modelID == "" {
				continue
			}
			ids = append(ids, modelID)
			for _, key := range []string{"compactionModel", "contextPromotionTarget"} {
				if ref, ok := model[key].(string); ok && strings.TrimSpace(ref) != "" {
					refs.InFile[providerID] = append(refs.InFile[providerID], strings.TrimSpace(ref))
				}
			}
		}
		refs.Models[providerID] = ids
	}
	return refs
}

// ompScanAliasReasons 扫描 provider / model 节点上的 YAML 别名与 merge 键。
// 命中时整份文件在 LoadModels 阶段就可能被拒（ErrReadOnlyNode），这里保留
// 逐 provider 的只读原因，供未来允许只读加载时使用。
func ompScanAliasReasons(data []byte) map[string]string {
	reasons := map[string]string{}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return reasons
	}
	root := &document
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return reasons
		}
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return reasons
	}
	providers := ompYAMLChild(root, "providers")
	if providers == nil || providers.Kind != yaml.MappingNode {
		return reasons
	}
	for i := 0; i+1 < len(providers.Content); i += 2 {
		id := strings.TrimSpace(providers.Content[i].Value)
		value := providers.Content[i+1]
		if id == "" {
			continue
		}
		if value.Kind == yaml.AliasNode {
			reasons[id] = "是 YAML 别名节点，P1 无法安全改写"
			continue
		}
		if value.Kind != yaml.MappingNode {
			continue
		}
		if ompYAMLHasMerge(value) {
			reasons[id] = "使用了 YAML merge 键（<<），P1 无法安全改写"
			continue
		}
		models := ompYAMLChild(value, "models")
		if models == nil || models.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range models.Content {
			if item.Kind == yaml.AliasNode {
				reasons[id] = "的模型使用了 YAML 别名节点，P1 无法安全改写"
				break
			}
			if item.Kind == yaml.MappingNode && ompYAMLHasMerge(item) {
				reasons[id] = "的模型使用了 YAML merge 键（<<），P1 无法安全改写"
				break
			}
		}
	}
	return reasons
}

func ompYAMLChild(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// ompRenameProviderFile 在 YAML 节点层面把 providers 的键 oldID 改成 newID，
// 再交给注入的解析器重新解析成新的文件句柄。
//
// 重命名必须走这条路径：ompprovider.SetProvider 只写 APM 管理的键，直接
// 「新增 + 删除旧 ID」会丢掉 provider 级的未知键（transport / compat 等）。
func ompRenameProviderFile(file OMPModelsRead, oldID, newID string) (OMPModelsRead, error) {
	data, err := file.Bytes()
	if err != nil {
		return nil, fmt.Errorf("序列化 omp 模型文件失败：%v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("解析 omp 模型文件失败：%v", err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("omp 模型文件缺少 providers 映射，无法重命名提供商「%s」", oldID)
	}
	providers := ompYAMLChild(document.Content[0], "providers")
	if providers == nil || providers.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("omp 模型文件缺少 providers 映射，无法重命名提供商「%s」", oldID)
	}
	found := false
	for i := 0; i+1 < len(providers.Content); i += 2 {
		key := providers.Content[i]
		if key.Kind != yaml.ScalarNode {
			continue
		}
		if key.Value == newID {
			return nil, fmt.Errorf("提供商「%s」已存在，不能重命名为此 ID", newID)
		}
		if key.Value == oldID {
			key.Value = newID
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("提供商「%s」不存在", oldID)
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, fmt.Errorf("生成 omp 模型文件失败：%v", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("生成 omp 模型文件失败：%v", err)
	}
	out := bytes.ReplaceAll(buf.Bytes(), []byte("\r\n"), []byte("\n"))
	if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	renamed, err := ompLoadModelsFile(out)
	if err != nil {
		return nil, fmt.Errorf("重命名后重新解析 omp 模型文件失败：%v", err)
	}
	return renamed, nil
}

func ompYAMLHasMerge(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "<<" {
			return true
		}
	}
	return false
}

// OMPSelectorsAtPath 读取指定 models 文件并生成 omp 模型角色选择器列表。
// 读不到文件或解析失败时返回空切片（App 层据此显示“暂无可选模型”），不报错。
func OMPSelectorsAtPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return []string{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	file, err := ompLoadModelsFile(data)
	if err != nil {
		return []string{}
	}
	return OMPSelectors(file)
}
