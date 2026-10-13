package dshprovider

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Patch 是一份 profile patch 文档，即 cordis.patch.yml 的顶层数组。
//
// 内部保留 yaml.Node 树，因此未被本包改写的条目（包括 `!!js` 表达式与注释）
// 在写回时原样保留；这与 DSH 自己的 config-editor 行为一致。
type Patch struct {
	seq *yaml.Node
}

// NewPatch 返回一份空的 patch（顶层 []）。
func NewPatch() *Patch {
	return &Patch{seq: &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}}
}

// LoadPatch 解析 patch 文档。空文件与只含注释的文件等价于空 patch。
func LoadPatch(data []byte) (*Patch, error) {
	if strings.TrimSpace(string(data)) == "" {
		return NewPatch(), nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("DeepSeek Harness profile patch 解析失败（YAML）：%v", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return NewPatch(), nil
	}
	root := doc.Content[0]
	switch root.Kind {
	case yaml.SequenceNode:
		return &Patch{seq: root}, nil
	case yaml.ScalarNode:
		if root.Tag == "!!null" || strings.TrimSpace(root.Value) == "" {
			return NewPatch(), nil
		}
	}
	return nil, fmt.Errorf("DeepSeek Harness profile patch 的根节点必须是数组 [...], 实际是 %s", nodeKindName(root))
}

// Len 返回顶层条目数量。
func (p *Patch) Len() int {
	if p == nil || p.seq == nil {
		return 0
	}
	return len(p.seq.Content)
}

// HasLLMPiAIEntry 表示 patch 里是否已经有 llm-pi-ai 条目。
func (p *Patch) HasLLMPiAIEntry() bool {
	return p.findRow(LLMPiAIEntryID) >= 0
}

// Providers 返回 llm-pi-ai 条目 config.providers 的解码副本。
// 条目不存在或没有 providers 时返回空映射。
func (p *Patch) Providers() (map[string]interface{}, error) {
	cfg, err := p.LLMPiAIConfig()
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	raw, ok := cfg["providers"]
	if !ok || raw == nil {
		return out, nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("llm-pi-ai 的 providers 必须是映射")
	}
	return m, nil
}

// LLMPiAIConfig 返回 llm-pi-ai 条目 config 解码后的副本。
func (p *Patch) LLMPiAIConfig() (map[string]interface{}, error) {
	out := map[string]interface{}{}
	idx := p.findRow(LLMPiAIEntryID)
	if idx < 0 {
		return out, nil
	}
	row := p.seq.Content[idx]
	cfgNode := mapValue(row, "config")
	if cfgNode == nil || cfgNode.Kind == yaml.ScalarNode && cfgNode.Tag == "!!null" {
		return out, nil
	}
	if cfgNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("llm-pi-ai 的 config 必须是映射")
	}
	if err := cfgNode.Decode(&out); err != nil {
		return nil, fmt.Errorf("llm-pi-ai 的 config 解析失败：%v", err)
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}

// SetProviders 写入 llm-pi-ai 条目的 config.providers。
// 已有条目就地替换 config（保留该条目其它 config 键与 name）；
// 没有条目时按 DSH config-editor 的做法追加一行 {id, name, config}。
func (p *Patch) SetProviders(providers map[string]interface{}) error {
	if providers == nil {
		providers = map[string]interface{}{}
	}
	idx := p.findRow(LLMPiAIEntryID)
	if idx < 0 {
		cfgNode := &yaml.Node{}
		if err := cfgNode.Encode(map[string]interface{}{"providers": providers}); err != nil {
			return fmt.Errorf("序列化 llm-pi-ai 配置失败：%v", err)
		}
		row := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMapValue(row, "id", scalarNode(LLMPiAIEntryID))
		setMapValue(row, "name", scalarNode(LLMPiAIPluginName))
		setMapValue(row, "config", cfgNode)
		p.seq.Content = append(p.seq.Content, row)
		return nil
	}
	row := p.seq.Content[idx]
	cfgNode := mapValue(row, "config")
	if cfgNode == nil || (cfgNode.Kind == yaml.ScalarNode && cfgNode.Tag == "!!null") {
		cfgNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMapValue(row, "config", cfgNode)
	}
	if cfgNode.Kind != yaml.MappingNode {
		return fmt.Errorf("llm-pi-ai 的 config 必须是映射")
	}
	// 只替换 config.providers 子树：config 里的其它键、注释与 !!js 表达式保持原样。
	// 旧实现把整个 config 解码成 map 再编码，会静默丢掉这些内容。
	provNode := mapValue(cfgNode, "providers")
	if provNode == nil || provNode.Kind != yaml.MappingNode {
		encoded := &yaml.Node{}
		if err := encoded.Encode(providers); err != nil {
			return fmt.Errorf("序列化 llm-pi-ai 配置失败：%v", err)
		}
		setMapValue(cfgNode, "providers", encoded)
		return nil
	}
	if err := syncMappingNode(provNode, providers); err != nil {
		return fmt.Errorf("序列化 llm-pi-ai 配置失败：%v", err)
	}
	return nil
}

// DeleteRow 删除最后一个匹配 id 且不带 insert 的顶层条目；返回是否删除成功。
func (p *Patch) DeleteRow(id string) bool {
	idx := p.findRow(id)
	if idx < 0 {
		return false
	}
	p.seq.Content = append(p.seq.Content[:idx], p.seq.Content[idx+1:]...)
	return true
}

// AgentDefaultModel 返回 agent-default-model 条目 config 的解码副本。
func (p *Patch) AgentDefaultModel() map[string]interface{} {
	idx := p.findRow(AgentDefaultModelEntryID)
	if idx < 0 {
		return nil
	}
	cfgNode := mapValue(p.seq.Content[idx], "config")
	if cfgNode == nil || cfgNode.Kind != yaml.MappingNode {
		return nil
	}
	var out map[string]interface{}
	if err := cfgNode.Decode(&out); err != nil {
		return nil
	}
	return out
}

// SetAgentDefaultModel 就地更新 agent-default-model 条目的 config（条目不存在时不做任何事）。
func (p *Patch) SetAgentDefaultModel(cfg map[string]interface{}) error {
	if cfg == nil {
		return nil
	}
	idx := p.findRow(AgentDefaultModelEntryID)
	if idx < 0 {
		return nil
	}
	row := p.seq.Content[idx]
	cur := mapValue(row, "config")
	if cur != nil && cur.Kind == yaml.MappingNode {
		// 就地更新：未被改动的键与其注释保持原样。
		if err := syncMappingNode(cur, cfg); err != nil {
			return fmt.Errorf("序列化 agent-default-model 配置失败：%v", err)
		}
		return nil
	}
	node := &yaml.Node{}
	if err := node.Encode(cfg); err != nil {
		return fmt.Errorf("序列化 agent-default-model 配置失败：%v", err)
	}
	setMapValue(row, "config", node)
	return nil
}

// Bytes 序列化 patch。空 patch 输出 "[]"，因为 DSH 拒绝空文件与只含注释的 patch。
func (p *Patch) Bytes() ([]byte, error) {
	if p == nil || p.seq == nil || len(p.seq.Content) == 0 {
		return []byte("[]\n"), nil
	}
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(p.seq); err != nil {
		return nil, fmt.Errorf("序列化 profile patch 失败：%v", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("序列化 profile patch 失败：%v", err)
	}
	out := sb.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}

// findRow 返回最后一个匹配 id 且不带 insert 的顶层条目下标，找不到返回 -1。
// 与 DSH 的 last-write-wins 语义一致。
func (p *Patch) findRow(id string) int {
	if p == nil || p.seq == nil {
		return -1
	}
	found := -1
	for i, item := range p.seq.Content {
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		if nodeScalar(mapValue(item, "id")) != id {
			continue
		}
		if mapValue(item, "insert") != nil {
			continue
		}
		found = i
	}
	return found
}

// DefinesLLMPiAIProviders 判断一份 patch 是否“定义了 llm-pi-ai 的 providers”。
// 用于检测 home patch 覆盖：被覆盖时 profile patch 的写入不会生效。
// 只有 llm-pi-ai 条目的 config 是映射且带 providers 键时才为 true（providers: {}
// 也算定义）；仅存在 config（如 config: {}）或 config 里没有 providers 时不算覆盖。
func DefinesLLMPiAIProviders(data []byte) (bool, error) {
	p, err := LoadPatch(data)
	if err != nil {
		return false, err
	}
	idx := p.findRow(LLMPiAIEntryID)
	if idx < 0 {
		return false, nil
	}
	cfg := mapValue(p.seq.Content[idx], "config")
	if cfg == nil || cfg.Kind != yaml.MappingNode {
		return false, nil
	}
	return mapValue(cfg, "providers") != nil, nil
}

// isPatchShapedOrEmpty 判断内容是否是一份 patch：顶层数组，或空/只有注释的
// 等价形态（DSH 把它们都当作空 patch）。解析失败返回 false，交给调用方回退。
func isPatchShapedOrEmpty(data []byte) bool {
	if len(data) == 0 || strings.TrimSpace(string(data)) == "" {
		return true
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return true
	}
	return doc.Content[0].Kind == yaml.SequenceNode
}
// looksLikePatchSequence 判断内容是否是一份顶层数组的 patch（用于格式探测）。
func looksLikePatchSequence(data []byte) bool {
	if len(data) == 0 || strings.TrimSpace(string(data)) == "" {
		return false
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return false
	}
	return doc.Content[0].Kind == yaml.SequenceNode
}

func nodeKindName(n *yaml.Node) string {
	if n == nil {
		return "空"
	}
	switch n.Kind {
	case yaml.DocumentNode:
		return "文档"
	case yaml.SequenceNode:
		return "数组"
	case yaml.MappingNode:
		return "映射"
	case yaml.ScalarNode:
		return "标量"
	case yaml.AliasNode:
		return "别名"
	}
	return "未知节点"
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// mapValue 返回映射节点中 key 对应的值节点。
func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// setMapValue 设置映射节点的键值；键不存在时追加。
func setMapValue(n *yaml.Node, key string, value *yaml.Node) {
	if n.Kind != yaml.MappingNode {
		n.Kind = yaml.MappingNode
		n.Tag = "!!map"
		n.Content = nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			n.Content[i+1] = value
			return
		}
	}
	n.Content = append(n.Content, scalarNode(key), value)
}

// syncMappingNode 把映射节点就地同步为 values：
//   - 值没变的键保留原节点，注释、!!js 标签与键序都不动；
//   - 值变了的键替换成按 values 重新编码的节点；
//   - values 里没有的键删除；
//   - 新增键按名字排序追加（保证输出稳定）。
func syncMappingNode(node *yaml.Node, values map[string]interface{}) error {
	content := make([]*yaml.Node, 0, len(node.Content))
	present := make(map[string]bool, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]
		present[key.Value] = true
		want, ok := values[key.Value]
		if !ok {
			continue
		}
		if mappingValueEqual(val, want) {
			content = append(content, key, val)
			continue
		}
		encoded := &yaml.Node{}
		if err := encoded.Encode(want); err != nil {
			return err
		}
		content = append(content, key, encoded)
	}
	added := make([]string, 0, len(values))
	for k := range values {
		if !present[k] {
			added = append(added, k)
		}
	}
	sort.Strings(added)
	for _, k := range added {
		encoded := &yaml.Node{}
		if err := encoded.Encode(values[k]); err != nil {
			return err
		}
		content = append(content, scalarNode(k), encoded)
	}
	node.Content = content
	return nil
}

// mappingValueEqual 判断现有值节点的解码结果是否与目标值一致。
// 用 JSON 序列化比较可以忽略 YAML→map→JSON 往返带来的 int/float64 差异；
// 一致时保留原节点，注释与 !!js 标签因此不会被写没。
func mappingValueEqual(node *yaml.Node, want interface{}) bool {
	var got interface{}
	if err := node.Decode(&got); err != nil {
		return false
	}
	gotJSON, gerr := json.Marshal(got)
	wantJSON, werr := json.Marshal(want)
	if gerr != nil || werr != nil {
		return false
	}
	return string(gotJSON) == string(wantJSON)
}

func nodeScalar(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(n.Value)
}
