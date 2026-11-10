package ompprovider

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFile 是 config.yml 的节点级文档模型，P1 只读写 modelRoles。
type ConfigFile struct {
	root *yaml.Node
}

// NewConfigFile 创建空配置文档。
func NewConfigFile() *ConfigFile { return &ConfigFile{} }

// LoadConfig 解析 config.yml 内容；空文件、纯注释、null 根都视为空文档。
func LoadConfig(data []byte) (*ConfigFile, error) {
	root, err := decodeDocument(data)
	if err != nil {
		return nil, err
	}
	return &ConfigFile{root: root}, nil
}

// ModelRoles 返回 modelRoles 的副本。
func (f *ConfigFile) ModelRoles() (map[string]string, error) {
	out := make(map[string]string)
	if f == nil || f.root == nil || f.root.Kind != yaml.MappingNode {
		return out, nil
	}
	idx := mappingIndex(f.root, "modelRoles")
	if idx < 0 {
		return out, nil
	}
	node := f.root.Content[idx+1]
	if node.Kind == yaml.AliasNode {
		return nil, fmt.Errorf("modelRoles %w", ErrReadOnlyNode)
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return out, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New("config.yml 的 modelRoles 必须是映射对象")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode {
			continue
		}
		value := node.Content[i+1]
		if value.Kind == yaml.AliasNode {
			return nil, fmt.Errorf("modelRoles.%s %w", key.Value, ErrReadOnlyNode)
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return nil, fmt.Errorf("modelRoles.%s 必须是字符串", key.Value)
		}
		out[key.Value] = value.Value
	}
	return out, nil
}

// SetModelRole 设置 modelRoles.<role>；selector 为空表示删除该键。
func (f *ConfigFile) SetModelRole(role, selector string) error {
	if f == nil {
		return errors.New("配置文档未初始化")
	}
	role = strings.TrimSpace(role)
	if role == "" {
		return errors.New("角色名不能为空")
	}
	if strings.ContainsAny(role, "*?/@") || strings.Contains(role, "/") {
		return fmt.Errorf("角色名 %q 不合法", role)
	}
	selector = strings.TrimSpace(selector)
	if selector != "" {
		if err := validateSelector(selector); err != nil {
			return err
		}
	}
	root := mappingRoot(&f.root)
	roles, err := ensureChildMapping(root, "modelRoles")
	if err != nil {
		return err
	}
	if selector == "" {
		deleteMappingKey(roles, role)
		return nil
	}
	setScalarStringNode(roles, role, selector)
	return nil
}

// Bytes 把文档编码成 UTF-8 + LF、2 空格缩进、末尾换行的 YAML，并保留注释。
func (f *ConfigFile) Bytes() ([]byte, error) {
	if f == nil {
		return nil, nil
	}
	return encodeDocumentRoot(f.root)
}

// validateSelector 校验 P1 支持的精确选择器（provider/model 可带 :effort）。
func validateSelector(selector string) error {
	if strings.ContainsAny(selector, "*?") {
		return fmt.Errorf("选择器 %q 含有通配符，P1 只接受精确选择器", selector)
	}
	if strings.HasPrefix(selector, "@") {
		return fmt.Errorf("选择器 %q 是 @role 别名，P1 只接受精确选择器", selector)
	}
	segments := strings.SplitN(selector, "/", 2)
	if len(segments) != 2 || strings.TrimSpace(segments[0]) == "" || strings.TrimSpace(segments[1]) == "" {
		return fmt.Errorf("选择器 %q 必须是 provider/model 形式", selector)
	}
	return nil
}
