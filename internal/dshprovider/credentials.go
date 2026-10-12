package dshprovider

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// CredentialsVersion 是 DSH 0.2 凭据文件的版本号。
// 0.1 的扁平布局会被 DSH 拒绝，因此这里写盘一律产出 version: 1 + refs 分节。
const CredentialsVersion = 1

var credentialRefName = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")

// LoadCredentialRefs 读取凭据文件里的 refs（环境变量名 -> 密钥）。
// 兼容 0.1 的扁平布局；文件不存在时返回空映射。
func LoadCredentialRefs(path string) (map[string]string, error) {
	root, err := loadCredentialRoot(path)
	if err != nil || root == nil {
		return map[string]string{}, err
	}
	refs := mapValue(root, "refs")
	if refs == nil {
		if mapValue(root, "version") != nil {
			return map[string]string{}, nil
		}
		refs = root // 0.1 扁平布局
	}
	if refs.Kind != yaml.MappingNode {
		return map[string]string{}, fmt.Errorf("凭据文件的 refs 必须是映射")
	}
	out := map[string]string{}
	for i := 0; i+1 < len(refs.Content); i += 2 {
		key := strings.TrimSpace(refs.Content[i].Value)
		val := refs.Content[i+1]
		if key == "" || val.Kind != yaml.ScalarNode {
			continue
		}
		if s, ok := scalarString(val); ok {
			out[key] = s
		}
	}
	return out, nil
}

// SetCredentialRef 写入或覆盖一条 ref。value 为空时等同于删除该 ref。
// 文件不存在时创建 version: 1 布局；0.1 扁平布局会被就地升级为 version 1。
func SetCredentialRef(path, name, value string) error {
	name = strings.TrimSpace(name)
	if !credentialRefName.MatchString(name) {
		return fmt.Errorf("凭据名「%s」必须是字母/数字/下划线组成的环境变量名", name)
	}
	if strings.TrimSpace(value) == "" {
		return RemoveCredentialRef(path, name)
	}
	root, err := loadCredentialRoot(path)
	if err != nil {
		return err
	}
	if root == nil {
		root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if err := ensureVersionedRoot(root); err != nil {
		return err
	}
	refs := mapValue(root, "refs")
	if refs == nil || refs.Kind != yaml.MappingNode {
		refs = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMapValue(root, "refs", refs)
	}
	refs.Content = setScalarEntry(refs.Content, name, scalarNode(strings.TrimSpace(value)))
	return writeCredentialRoot(path, root)
}

// RemoveCredentialRef 删除一条 ref；文件或 ref 不存在时是空操作。
func RemoveCredentialRef(path, name string) error {
	root, err := loadCredentialRoot(path)
	if err != nil || root == nil {
		return err
	}
	if mapValue(root, "version") == nil {
		// 0.1 扁平布局：直接作用在根上。
		root.Content = deleteScalarEntry(root.Content, strings.TrimSpace(name))
		return writeCredentialRoot(path, root)
	}
	refs := mapValue(root, "refs")
	if refs == nil {
		return nil
	}
	refs.Content = deleteScalarEntry(refs.Content, strings.TrimSpace(name))
	return writeCredentialRoot(path, root)
}

// ensureVersionedRoot 把 0.1 的扁平布局升级为 version: 1 + refs 分节。
func ensureVersionedRoot(root *yaml.Node) error {
	if v := mapValue(root, "version"); v != nil {
		if nodeScalar(v) != fmt.Sprint(CredentialsVersion) {
			return fmt.Errorf("凭据文件声明 version %s，本程序只支持 version %d", nodeScalar(v), CredentialsVersion)
		}
		return nil
	}
	flat := &yaml.Node{Kind: root.Kind, Tag: root.Tag, Content: root.Content}
	root.Content = nil
	setMapValue(root, "version", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(CredentialsVersion)})
	setMapValue(root, "refs", flat)
	return nil
}

func loadCredentialRoot(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 DeepSeek 凭据文件失败：%v", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("DeepSeek 凭据文件解析失败（%v）", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("DeepSeek 凭据文件的根节点必须是映射")
	}
	return root, nil
}

func writeCredentialRoot(path string, root *yaml.Node) error {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("创建凭据目录失败：%v", err)
		}
	}
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return fmt.Errorf("序列化 DeepSeek 凭据失败：%v", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("序列化 DeepSeek 凭据失败：%v", err)
	}
	out := sb.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	d := dir
	if d == "" {
		d = "."
	}
	tmp, err := os.CreateTemp(d, ".credentials.tmp_*")
	if err != nil {
		return fmt.Errorf("写入凭据文件失败：%v", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(out); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("写入凭据文件失败：%v", err)
	}
	_ = tmp.Sync()
	tmp.Close()
	_ = os.Chmod(tmpName, 0600)
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("写入凭据文件失败：%v", err)
	}
	return nil
}

func setScalarEntry(content []*yaml.Node, key string, value *yaml.Node) []*yaml.Node {
	for i := 0; i+1 < len(content); i += 2 {
		if content[i].Value == key {
			content[i+1] = value
			return content
		}
	}
	return append(content, scalarNode(key), value)
}

func deleteScalarEntry(content []*yaml.Node, key string) []*yaml.Node {
	out := content[:0]
	for i := 0; i+1 < len(content); i += 2 {
		if content[i].Value == key {
			continue
		}
		out = append(out, content[i], content[i+1])
	}
	return out
}

func scalarString(n *yaml.Node) (string, bool) {
	switch n.Tag {
	case "!!str", "":
		return n.Value, true
	case "!!int", "!!float", "!!bool":
		return n.Value, true
	}
	return "", false
}
