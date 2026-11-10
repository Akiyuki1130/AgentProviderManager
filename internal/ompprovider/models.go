package ompprovider

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// 文档级防御上限：超过即拒绝解析，避免异常文件把界面卡死。
const (
	maxDocumentBytes = 8 << 20
	maxNodeDepth     = 64
)

var (
	// ErrModelsIsJSONOnly 表示目标目录只有 models.json，尚未完成 omp 迁移，P1 不允许写入。
	ErrModelsIsJSONOnly = errors.New("当前只有 models.json，请先运行一次 omp 完成迁移")
	// ErrFileTooLarge 表示待解析内容超过大小上限。
	ErrFileTooLarge = errors.New("配置文件超过大小上限")
	// ErrMultiDocument 表示文件包含多份 YAML 文档。
	ErrMultiDocument = errors.New("不支持多文档 YAML（--- 分隔）")
	// ErrRootNotMapping 表示根节点不是映射对象。
	ErrRootNotMapping = errors.New("根节点必须是映射对象")
	// ErrDuplicateKey 表示同一映射内出现重复键。
	ErrDuplicateKey = errors.New("存在重复键")
	// ErrDepthExceeded 表示 YAML 嵌套过深。
	ErrDepthExceeded = errors.New("YAML 嵌套层级过深")
	// ErrReadOnlyNode 表示节点是 YAML 别名或使用 merge 键，P1 只读。
	ErrReadOnlyNode = errors.New("含 YAML 别名或 merge 键，P1 只读")
)

// ModelsFile 是 models.yml 的节点级文档模型，保留注释、键顺序与自定义标签。
type ModelsFile struct {
	root *yaml.Node
}

// NewModelsFile 创建空文档。
func NewModelsFile() *ModelsFile { return &ModelsFile{} }

// LoadModels 解析 models.yml / models.json 内容；空文件、纯注释、null 根都视为空文档。
func LoadModels(data []byte) (*ModelsFile, error) {
	root, err := decodeDocument(data)
	if err != nil {
		return nil, err
	}
	return &ModelsFile{root: root}, nil
}

// Providers 返回 providers 的深拷贝；调用方拿不到 *yaml.Node。
func (f *ModelsFile) Providers() (map[string]map[string]interface{}, error) {
	out := make(map[string]map[string]interface{})
	pn := f.providersNode()
	if pn == nil {
		return out, nil
	}
	for i := 0; i+1 < len(pn.Content); i += 2 {
		key := pn.Content[i]
		val := pn.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return nil, errors.New("providers 的键必须是字符串")
		}
		if val.Kind == yaml.ScalarNode && val.Tag == "!!null" {
			out[key.Value] = map[string]interface{}{}
			continue
		}
		if val.Kind != yaml.MappingNode && val.Kind != yaml.AliasNode {
			return nil, fmt.Errorf("provider %q 必须是映射对象", key.Value)
		}
		var raw map[string]interface{}
		if err := val.Decode(&raw); err != nil {
			return nil, fmt.Errorf("解析 provider %q 失败：%w", key.Value, err)
		}
		if raw == nil {
			raw = map[string]interface{}{}
		}
		out[key.Value] = raw
	}
	return out, nil
}

// ProviderIDs 按文件顺序返回 provider ID。
func (f *ModelsFile) ProviderIDs() []string {
	pn := f.providersNode()
	if pn == nil {
		return nil
	}
	ids := make([]string, 0, len(pn.Content)/2)
	for i := 0; i+1 < len(pn.Content); i += 2 {
		if key := pn.Content[i]; key.Kind == yaml.ScalarNode {
			ids = append(ids, key.Value)
		}
	}
	return ids
}

// HasProvider 报告指定 provider 是否存在。
func (f *ModelsFile) HasProvider(id string) bool {
	pn := f.providersNode()
	return pn != nil && mappingIndex(pn, id) >= 0
}

// Bytes 把文档编码成 UTF-8 + LF、2 空格缩进、末尾换行的 YAML，并保留注释。
func (f *ModelsFile) Bytes() ([]byte, error) {
	if f == nil {
		return nil, nil
	}
	return encodeDocumentRoot(f.root)
}

// providersNode 返回 providers 映射节点；不存在或形状不对时返回 nil。
func (f *ModelsFile) providersNode() *yaml.Node {
	if f == nil || f.root == nil || f.root.Kind != yaml.MappingNode {
		return nil
	}
	idx := mappingIndex(f.root, "providers")
	if idx < 0 {
		return nil
	}
	n := f.root.Content[idx+1]
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

// mappingRoot 保证 root 是映射节点。
func mappingRoot(root **yaml.Node) *yaml.Node {
	if *root == nil || (*root).Kind != yaml.MappingNode {
		*root = newMappingNode()
	}
	return *root
}

// ---- 节点基础工具 ----

func newMappingNode() *yaml.Node  { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }
func newSequenceNode() *yaml.Node { return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"} }

func newStringNode(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func newBoolNode(v bool) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"}
	if v {
		n.Value = "true"
	}
	return n
}

func newIntNode(v int64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(v, 10)}
}

func newFloatNode(v float64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(v, 'g', -1, 64)}
}

// mappingIndex 返回键在映射节点 Content 中的下标（键节点位置），找不到返回 -1。
func mappingIndex(n *yaml.Node, key string) int {
	if n == nil || n.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		if k.Kind == yaml.ScalarNode && k.Tag != "!!merge" && k.Value == key {
			return i
		}
	}
	return -1
}

// lookupChild 返回键对应的值节点。
func lookupChild(n *yaml.Node, key string) (*yaml.Node, bool) {
	idx := mappingIndex(n, key)
	if idx < 0 {
		return nil, false
	}
	return n.Content[idx+1], true
}

// deleteMappingAt 删除 Content 中下标 i 的键值对。
func deleteMappingAt(n *yaml.Node, i int) {
	if n == nil || i < 0 || i+1 >= len(n.Content) {
		return
	}
	n.Content = append(n.Content[:i], n.Content[i+2:]...)
}

// deleteMappingKey 删除指定键，返回是否删除成功。
func deleteMappingKey(n *yaml.Node, key string) bool {
	idx := mappingIndex(n, key)
	if idx < 0 {
		return false
	}
	deleteMappingAt(n, idx)
	return true
}

// scalarStringValue 把标量节点读成字符串；null 不算。
func scalarStringValue(n *yaml.Node) (string, bool) {
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag == "!!null" {
		return "", false
	}
	return n.Value, true
}

// numberValue 把标量节点读成数值。
func numberValue(n *yaml.Node) (float64, bool) {
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag == "!!null" {
		return 0, false
	}
	var f float64
	if err := n.Decode(&f); err != nil {
		return 0, false
	}
	return f, true
}

// copyComments 把旧节点上的注释带到替换后的新节点上。
func copyComments(dst, src *yaml.Node) {
	if dst == nil || src == nil {
		return
	}
	dst.HeadComment = src.HeadComment
	dst.LineComment = src.LineComment
	dst.FootComment = src.FootComment
}

// hasAliasOrMerge 报告子树里是否含 YAML 别名或 merge 键。
func hasAliasOrMerge(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.Kind == yaml.AliasNode {
		return true
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Tag == "!!merge" {
				return true
			}
			if hasAliasOrMerge(n.Content[i+1]) {
				return true
			}
		}
		return false
	}
	for _, child := range n.Content {
		if hasAliasOrMerge(child) {
			return true
		}
	}
	return false
}

// ---- 解析 ----

// decodeDocument 把内容解析成单个 YAML 文档的根映射节点；空文档返回 nil。
func decodeDocument(data []byte) (*yaml.Node, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(data) > maxDocumentBytes {
		return nil, fmt.Errorf("%w（共 %d 字节）", ErrFileTooLarge, len(data))
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("YAML 解析失败：%w", err)
	}
	if err := ensureSingleDocument(decoder); err != nil {
		return nil, err
	}
	root := documentRoot(&doc)
	if root == nil {
		return nil, nil
	}
	if root.Kind != yaml.MappingNode {
		return nil, ErrRootNotMapping
	}
	if err := checkDocumentDepth(root, 0); err != nil {
		return nil, err
	}
	if err := checkDuplicateKeys(root, ""); err != nil {
		return nil, err
	}
	return root, nil
}

// ensureSingleDocument 确认后续只有空文档；出现第二份真实文档即报错。
func ensureSingleDocument(decoder *yaml.Decoder) error {
	for {
		var next yaml.Node
		err := decoder.Decode(&next)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("YAML 解析失败：%w", err)
		}
		if isEmptyDocument(&next) {
			continue
		}
		return ErrMultiDocument
	}
}

// isEmptyDocument 判断文档节点是否为空（只有 null 或什么都没有）。
func isEmptyDocument(doc *yaml.Node) bool {
	n := doc
	if n == nil {
		return true
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return true
		}
		n = n.Content[0]
	}
	if n == nil {
		return true
	}
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// documentRoot 取出文档的内容节点；null 文档返回 nil。
func documentRoot(doc *yaml.Node) *yaml.Node {
	n := doc
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nil
		}
		n = n.Content[0]
	}
	if n == nil {
		return nil
	}
	if n.Kind == yaml.ScalarNode && (n.Tag == "!!null" || (n.Tag == "" && strings.TrimSpace(n.Value) == "")) {
		return nil
	}
	return n
}

// checkDocumentDepth 防御异常深嵌套。
func checkDocumentDepth(n *yaml.Node, depth int) error {
	if n == nil {
		return nil
	}
	if depth > maxNodeDepth {
		return ErrDepthExceeded
	}
	for _, child := range n.Content {
		if err := checkDocumentDepth(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// checkDuplicateKeys 递归检测重复键（yaml.v3 解码到 Node 时不报错，必须自己拦）。
func checkDuplicateKeys(n *yaml.Node, path string) error {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.DocumentNode:
		for _, child := range n.Content {
			if err := checkDuplicateKeys(child, path); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := make(map[string]bool, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			name := key.Value
			if key.Kind == yaml.ScalarNode {
				if seen[name] {
					return fmt.Errorf("%w：%s", ErrDuplicateKey, joinPath(path, name))
				}
				seen[name] = true
			}
			if err := checkDuplicateKeys(n.Content[i+1], joinPath(path, name)); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for i, child := range n.Content {
			if err := checkDuplicateKeys(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// joinPath 拼接错误路径。
func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	if key == "" {
		return parent
	}
	return parent + "." + key
}

// ---- 序列化 ----

// encodeDocumentRoot 把根节点编码成规范 YAML；空映射返回空内容。
func encodeDocumentRoot(root *yaml.Node) ([]byte, error) {
	if root == nil {
		return nil, nil
	}
	if root.Kind == yaml.MappingNode && len(root.Content) == 0 &&
		root.HeadComment == "" && root.LineComment == "" && root.FootComment == "" {
		return nil, nil
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return nil, fmt.Errorf("生成 YAML 失败：%w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("生成 YAML 失败：%w", err)
	}
	out := bytes.ReplaceAll(buf.Bytes(), []byte("\r\n"), []byte("\n"))
	if len(out) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	if len(out) > 0 {
		if _, err := decodeDocument(out); err != nil {
			return nil, fmt.Errorf("生成的 YAML 无法重新解析：%w", err)
		}
	}
	return out, nil
}

// ---- 值 <-> 节点 ----

// buildValueNode 把 Go 值构造成 yaml.Node；映射按键名排序，保证输出稳定。
func buildValueNode(value interface{}) (*yaml.Node, error) {
	switch typed := value.(type) {
	case string:
		return newStringNode(typed), nil
	case bool:
		return newBoolNode(typed), nil
	case int:
		return newIntNode(int64(typed)), nil
	case int32:
		return newIntNode(int64(typed)), nil
	case int64:
		return newIntNode(typed), nil
	case uint64:
		return newIntNode(int64(typed)), nil
	case float32:
		return newFloatNode(float64(typed)), nil
	case float64:
		if math.Trunc(typed) == typed && math.Abs(typed) < 1e15 {
			return newIntNode(int64(typed)), nil
		}
		return newFloatNode(typed), nil
	case []string:
		node := newSequenceNode()
		for _, item := range typed {
			node.Content = append(node.Content, newStringNode(item))
		}
		return node, nil
	case []interface{}:
		node := newSequenceNode()
		for _, item := range typed {
			child, err := buildValueNode(item)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case []map[string]interface{}:
		node := newSequenceNode()
		for _, item := range typed {
			child, err := buildValueNode(item)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case map[string]string:
		node := newMappingNode()
		for _, key := range sortedMapKeys(typed) {
			node.Content = append(node.Content, newStringNode(key), newStringNode(typed[key]))
		}
		return node, nil
	case map[string]interface{}:
		node := newMappingNode()
		for _, key := range sortedMapKeys(typed) {
			child, err := buildValueNode(typed[key])
			if err != nil {
				return nil, fmt.Errorf("%s：%w", key, err)
			}
			node.Content = append(node.Content, newStringNode(key), child)
		}
		return node, nil
	}
	return nil, fmt.Errorf("不支持的值类型 %T", value)
}

// nodeEqualsValue 判断节点与期望值是否语义相同（数值统一按浮点比较）。
func nodeEqualsValue(node *yaml.Node, value interface{}) bool {
	if node == nil || node.Kind == yaml.AliasNode {
		return false
	}
	var decoded interface{}
	if err := node.Decode(&decoded); err != nil {
		return false
	}
	return reflect.DeepEqual(normalizeValue(decoded), normalizeValue(value))
}

// normalizeValue 把值归一成 map[string]interface{} + float64 的形状，便于比较。
func normalizeValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = normalizeValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = normalizeValue(item)
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = normalizeValue(item)
		}
		return out
	case []string:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case uint64:
		return float64(typed)
	case float32:
		return float64(typed)
	}
	return value
}

// setNodeValue 在映射节点上设置键值：nil 表示删除；语义相同则保留原节点（保住标签）。
func setNodeValue(mapping *yaml.Node, key string, value interface{}) error {
	idx := mappingIndex(mapping, key)
	if value == nil {
		if idx >= 0 {
			deleteMappingAt(mapping, idx)
		}
		return nil
	}
	if idx >= 0 {
		existing := mapping.Content[idx+1]
		if nodeEqualsValue(existing, value) {
			return nil
		}
		next, err := buildValueNode(value)
		if err != nil {
			return fmt.Errorf("%s：%w", key, err)
		}
		copyComments(next, existing)
		mapping.Content[idx+1] = next
		return nil
	}
	next, err := buildValueNode(value)
	if err != nil {
		return fmt.Errorf("%s：%w", key, err)
	}
	mapping.Content = append(mapping.Content, newStringNode(key), next)
	return nil
}

// setScalarStringNode 写入字符串标量：文本相同则保留原节点（保住 !command / !!js 等标签）。
func setScalarStringNode(mapping *yaml.Node, key, value string) {
	idx := mappingIndex(mapping, key)
	if idx >= 0 {
		existing := mapping.Content[idx+1]
		if existing.Kind == yaml.ScalarNode && existing.Tag != "!!null" && existing.Value == value {
			return
		}
		next := newStringNode(value)
		copyComments(next, existing)
		mapping.Content[idx+1] = next
		return
	}
	mapping.Content = append(mapping.Content, newStringNode(key), newStringNode(value))
}

// sortedMapKeys 返回排序后的键名。
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
