package ompprovider

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// providerManagedKeyOrder 是 APM 管理的 provider 字段写入顺序。
var providerManagedKeyOrder = []string{"baseUrl", "apiKey", "api", "auth", "authHeader", "headers", "models"}

// modelKeyOrder 是模型字段的推荐写入顺序，其余键按名排序追加在末尾。
var modelKeyOrder = []string{
	"id", "name", "api", "kind", "baseUrl", "reasoning", "thinking", "input",
	"supportsTools", "contextWindow", "maxContextWindow", "maxTokens", "cost",
	"headers", "compat", "tokenizer", "promptCache", "contextPromotionTarget", "compactionModel",
}

// SetProvider 以节点级方式写入一个 provider；desired 只应包含 APM 管理的键。
//
// 语义约定：
//   - 值为 nil 表示删除该键；
//   - 未出现在 desired 里的键保留原节点（包括注释与未知字段）；
//   - 标量文本与现有节点相同则保留原节点，从而保住 !command / !!js 等自定义标签；
//   - apiKey 为空串表示「不覆盖」，用于界面未改动密钥的场景；
//   - headers 为空 map 表示删除 headers；
//   - mergeModels 为 true 时按 id 合并模型，为 false 时删除未列出的模型。
func (f *ModelsFile) SetProvider(id string, desired map[string]interface{}, mergeModels bool) error {
	if f == nil {
		return errors.New("文档未初始化")
	}
	if err := validateProviderID(id); err != nil {
		return err
	}
	if len(desired) == 0 {
		return nil
	}
	root := mappingRoot(&f.root)
	providers, err := ensureChildMapping(root, "providers")
	if err != nil {
		return err
	}

	created := false
	providerIdx := mappingIndex(providers, id)
	var provider *yaml.Node
	if providerIdx < 0 {
		provider = newMappingNode()
		providers.Content = append(providers.Content, newStringNode(id), provider)
		created = true
	} else {
		provider = providers.Content[providerIdx+1]
		switch provider.Kind {
		case yaml.AliasNode:
			return fmt.Errorf("provider %q %w", id, ErrReadOnlyNode)
		case yaml.MappingNode:
			if hasAliasOrMerge(provider) {
				return fmt.Errorf("provider %q %w", id, ErrReadOnlyNode)
			}
		case yaml.ScalarNode:
			if provider.Tag != "!!null" {
				return fmt.Errorf("provider %q 不是映射对象", id)
			}
			provider = newMappingNode()
			providers.Content[providerIdx+1] = provider
		default:
			return fmt.Errorf("provider %q 不是映射对象", id)
		}
	}

	for _, key := range providerManagedKeyOrder {
		if key == "models" {
			continue
		}
		value, ok := desired[key]
		if !ok {
			continue
		}
		if err := setProviderField(provider, key, value); err != nil {
			return fmt.Errorf("provider %q：%w", id, err)
		}
	}
	if value, ok := desired["models"]; ok {
		if err := setProviderModels(provider, value, mergeModels); err != nil {
			return fmt.Errorf("provider %q：%w", id, err)
		}
	}

	// 空 provider 在 omp 里是非法的，直接移除，避免写出无效文件。
	if len(provider.Content) == 0 && (created || providerIdx >= 0) {
		idx := mappingIndex(providers, id)
		if idx >= 0 {
			deleteMappingAt(providers, idx)
		}
	}
	return nil
}

// DeleteProvider 删除整个 provider，返回是否删除成功。
func (f *ModelsFile) DeleteProvider(id string) bool {
	providers := f.providersNode()
	if providers == nil {
		return false
	}
	idx := mappingIndex(providers, id)
	if idx < 0 {
		return false
	}
	deleteMappingAt(providers, idx)
	return true
}

// DeleteModel 删除指定模型；删到空时会移除 models 键并检查 provider 是否仍满足 omp 要求。
func (f *ModelsFile) DeleteModel(providerID, modelID string) (bool, error) {
	providers := f.providersNode()
	if providers == nil {
		return false, nil
	}
	providerIdx := mappingIndex(providers, providerID)
	if providerIdx < 0 {
		return false, nil
	}
	provider := providers.Content[providerIdx+1]
	if provider.Kind == yaml.AliasNode {
		return false, fmt.Errorf("provider %q %w", providerID, ErrReadOnlyNode)
	}
	if provider.Kind != yaml.MappingNode {
		return false, fmt.Errorf("provider %q 不是映射对象", providerID)
	}
	if hasAliasOrMerge(provider) {
		return false, fmt.Errorf("provider %q %w", providerID, ErrReadOnlyNode)
	}
	modelsIdx := mappingIndex(provider, "models")
	if modelsIdx < 0 {
		return false, nil
	}
	modelsNode := provider.Content[modelsIdx+1]
	if modelsNode.Kind != yaml.SequenceNode {
		return false, nil
	}

	target := -1
	for i, element := range modelsNode.Content {
		if element.Kind == yaml.AliasNode {
			return false, fmt.Errorf("provider %q 的 models[%d] %w", providerID, i, ErrReadOnlyNode)
		}
		if element.Kind != yaml.MappingNode {
			continue
		}
		idNode, ok := lookupChild(element, "id")
		if !ok {
			continue
		}
		id, isString := scalarStringValue(idNode)
		if isString && id == modelID {
			target = i
			break
		}
	}
	if target < 0 {
		return false, nil
	}
	if len(modelsNode.Content) == 1 && !providerHasG2Field(provider) {
		return false, fmt.Errorf("删除后 provider %q 的 models 为空且缺少 omp 必需字段（baseUrl、apiKey、auth: none 等），请整体删除该 provider", providerID)
	}
	modelsNode.Content = append(modelsNode.Content[:target], modelsNode.Content[target+1:]...)
	if len(modelsNode.Content) == 0 {
		deleteMappingAt(provider, modelsIdx)
	}
	return true, nil
}

// setProviderField 写入单个 provider 字段。
func setProviderField(provider *yaml.Node, key string, value interface{}) error {
	switch key {
	case "baseUrl", "api", "auth":
		if value == nil {
			deleteMappingKey(provider, key)
			return nil
		}
		text, err := stringField(value, key)
		if err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			deleteMappingKey(provider, key)
			return nil
		}
		setScalarStringNode(provider, key, text)
		return nil
	case "apiKey":
		if value == nil {
			deleteMappingKey(provider, key)
			return nil
		}
		text, err := stringField(value, key)
		if err != nil {
			return err
		}
		// 空密钥表示「不覆盖」：界面留空时保住文件里已有的值（可能是 !command 或环境变量名）。
		if text == "" {
			return nil
		}
		setScalarStringNode(provider, key, text)
		return nil
	case "authHeader":
		if value == nil {
			deleteMappingKey(provider, key)
			return nil
		}
		flag, ok := value.(bool)
		if !ok {
			return errors.New("authHeader 必须是布尔值")
		}
		if !flag {
			deleteMappingKey(provider, key)
			return nil
		}
		return setNodeValue(provider, key, true)
	case "headers":
		return setProviderHeaders(provider, value)
	}
	return fmt.Errorf("不支持的 provider 字段 %q", key)
}

// setProviderHeaders 以节点级方式写入 headers。
func setProviderHeaders(provider *yaml.Node, value interface{}) error {
	headers, err := toStringMap(value)
	if err != nil {
		return err
	}
	idx := mappingIndex(provider, "headers")
	if len(headers) == 0 {
		if idx >= 0 {
			deleteMappingAt(provider, idx)
		}
		return nil
	}
	var node *yaml.Node
	if idx >= 0 {
		node = provider.Content[idx+1]
		if node.Kind == yaml.AliasNode {
			return fmt.Errorf("headers %w", ErrReadOnlyNode)
		}
		if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
			node = newMappingNode()
			provider.Content[idx+1] = node
		}
		if node.Kind != yaml.MappingNode {
			return errors.New("headers 必须是映射对象")
		}
	} else {
		node = newMappingNode()
		provider.Content = append(provider.Content, newStringNode("headers"), node)
	}
	for _, key := range sortedMapKeys(headers) {
		if strings.TrimSpace(key) == "" {
			return errors.New("headers 的键不能为空")
		}
		setScalarStringNode(node, key, headers[key])
	}
	keep := make(map[string]bool, len(headers))
	for key := range headers {
		keep[key] = true
	}
	for i := 0; i+1 < len(node.Content); {
		if !keep[node.Content[i].Value] {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			continue
		}
		i += 2
	}
	return nil
}

// setProviderModels 以节点级方式写入 models 数组。
func setProviderModels(provider *yaml.Node, value interface{}, merge bool) error {
	models, err := toModelList(value)
	if err != nil {
		return err
	}
	idx := mappingIndex(provider, "models")
	if len(models) == 0 {
		if merge {
			return nil
		}
		if idx >= 0 {
			deleteMappingAt(provider, idx)
		}
		return nil
	}
	var seq *yaml.Node
	if idx < 0 {
		seq = newSequenceNode()
		provider.Content = append(provider.Content, newStringNode("models"), seq)
	} else {
		seq = provider.Content[idx+1]
		if seq.Kind == yaml.AliasNode {
			return fmt.Errorf("models %w", ErrReadOnlyNode)
		}
		if seq.Kind == yaml.ScalarNode && seq.Tag == "!!null" {
			seq = newSequenceNode()
			provider.Content[idx+1] = seq
		}
		if seq.Kind != yaml.SequenceNode {
			return errors.New("models 必须是数组")
		}
	}

	existing := make(map[string]int, len(seq.Content))
	for i, element := range seq.Content {
		if element.Kind == yaml.AliasNode {
			return fmt.Errorf("models[%d] %w", i, ErrReadOnlyNode)
		}
		if element.Kind != yaml.MappingNode {
			continue
		}
		idNode, ok := lookupChild(element, "id")
		if !ok {
			continue
		}
		if id, isString := scalarStringValue(idNode); isString && id != "" {
			existing[id] = i
		}
	}

	desired := make(map[string]bool, len(models))
	for _, model := range models {
		id, err := modelID(model)
		if err != nil {
			return err
		}
		desired[id] = true
		if pos, ok := existing[id]; ok {
			if err := mergeModelNode(seq.Content[pos], model); err != nil {
				return fmt.Errorf("模型 %q：%w", id, err)
			}
			continue
		}
		node, err := buildModelNode(model)
		if err != nil {
			return fmt.Errorf("模型 %q：%w", id, err)
		}
		seq.Content = append(seq.Content, node)
	}
	if merge {
		return nil
	}
	kept := make([]*yaml.Node, 0, len(seq.Content))
	for _, element := range seq.Content {
		id := ""
		if element.Kind == yaml.MappingNode {
			if idNode, ok := lookupChild(element, "id"); ok {
				id, _ = scalarStringValue(idNode)
			}
		}
		if desired[id] {
			kept = append(kept, element)
		}
	}
	seq.Content = kept
	return nil
}

// buildModelNode 构造新模型节点。
func buildModelNode(model map[string]interface{}) (*yaml.Node, error) {
	node := newMappingNode()
	if err := mergeModelNode(node, model); err != nil {
		return nil, err
	}
	return node, nil
}

// mergeModelNode 把模型字段合并进节点：nil 删键，未提到的键保留原节点。
func mergeModelNode(node *yaml.Node, model map[string]interface{}) error {
	for _, key := range orderedKeys(model, modelKeyOrder) {
		if err := setNodeValue(node, key, model[key]); err != nil {
			return err
		}
	}
	return nil
}

// modelID 取出并校验模型 id。
func modelID(model map[string]interface{}) (string, error) {
	value, ok := model["id"]
	if !ok {
		return "", errors.New("模型缺少 id")
	}
	text, ok := value.(string)
	if !ok {
		return "", errors.New("模型的 id 必须是字符串")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("模型的 id 不能为空")
	}
	return text, nil
}

// ensureChildMapping 取出子映射；缺失或为 null 时创建空映射。
func ensureChildMapping(parent *yaml.Node, key string) (*yaml.Node, error) {
	idx := mappingIndex(parent, key)
	if idx < 0 {
		node := newMappingNode()
		parent.Content = append(parent.Content, newStringNode(key), node)
		return node, nil
	}
	node := parent.Content[idx+1]
	if node.Kind == yaml.AliasNode {
		return nil, fmt.Errorf("%s %w", key, ErrReadOnlyNode)
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		node = newMappingNode()
		parent.Content[idx+1] = node
		return node, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s 必须是映射对象", key)
	}
	return node, nil
}

// providerHasG2Field 判断 provider 在 models 为空时是否仍满足 omp 的「非空 provider」要求。
func providerHasG2Field(provider *yaml.Node) bool {
	for _, key := range []string{"baseUrl", "apiKey", "guardrailIdentifier"} {
		if node, ok := lookupChild(provider, key); ok {
			if text, isString := scalarStringValue(node); isString && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	if node, ok := lookupChild(provider, "auth"); ok {
		if text, isString := scalarStringValue(node); isString && text == "none" {
			return true
		}
	}
	if node, ok := lookupChild(provider, "disableStrictTools"); ok {
		if flag, isBool := boolValue(node); isBool && flag {
			return true
		}
	}
	for _, key := range []string{"headers", "compat", "requestMetadata", "remoteCompaction", "modelOverrides", "discovery"} {
		if node, ok := lookupChild(provider, key); ok && node.Kind == yaml.MappingNode && len(node.Content) > 0 {
			return true
		}
	}
	return false
}

// validateProviderID 校验新建 provider 的 ID 形状。
func validateProviderID(id string) error {
	if id == "" {
		return errors.New("provider ID 不能为空")
	}
	if strings.TrimSpace(id) != id {
		return fmt.Errorf("provider ID %q 首尾不能有空白", id)
	}
	if strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("provider ID %q 不能包含斜杠", id)
	}
	for _, r := range id {
		allowed := r == '.' || r == '_' || r == '-' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !allowed {
			return fmt.Errorf("provider ID %q 含有非法字符 %q（只允许字母、数字、点、下划线和短横线）", id, string(r))
		}
	}
	return nil
}

// boolValue 把标量节点读成布尔值。
func boolValue(node *yaml.Node) (bool, bool) {
	if node == nil || node.Kind != yaml.ScalarNode {
		return false, false
	}
	var value bool
	if err := node.Decode(&value); err != nil {
		return false, false
	}
	return value, true
}

// stringField 读取字符串字段。
func stringField(value interface{}, field string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s 必须是字符串", field)
	}
	return text, nil
}

// toStringMap 把 headers 归一成 map[string]string。
func toStringMap(value interface{}) (map[string]string, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]string:
		return typed, nil
	case map[string]interface{}:
		out := make(map[string]string, len(typed))
		for key, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("headers 的值必须是字符串（键 %q 是 %T）", key, item)
			}
			out[key] = text
		}
		return out, nil
	}
	return nil, fmt.Errorf("headers 必须是 map[string]string，实际是 %T", value)
}

// toModelList 把 models 归一成 []map[string]interface{}。
func toModelList(value interface{}) ([]map[string]interface{}, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []map[string]interface{}:
		return typed, nil
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for i, item := range typed {
			mapping, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("models[%d] 必须是映射对象，实际是 %T", i, item)
			}
			out = append(out, mapping)
		}
		return out, nil
	}
	return nil, fmt.Errorf("models 必须是 []map[string]interface{}，实际是 %T", value)
}

// orderedKeys 先按推荐顺序输出，再把其余键按名排序追加。
func orderedKeys(mapping map[string]interface{}, order []string) []string {
	seen := make(map[string]bool, len(mapping))
	keys := make([]string, 0, len(mapping))
	for _, key := range order {
		if _, ok := mapping[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	rest := make([]string, 0, len(mapping))
	for key := range mapping {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sortStrings(rest)
	return append(keys, rest...)
}

// sortStrings 就地排序，避免再引一个导入。
func sortStrings(items []string) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
