package zcodeprovider

import (
	"fmt"
)

// Encode 把 Config 序列化为 canonical 形状的 JSON（2 空格缩进 + 末尾换行）。
// 编码前会跑一遍严格校验，确保只写出 schema 允许的键：否则 ZCode 的
// .strict() 校验会失败并把整份配置当作空。
//
// 键顺序与 ZCode 自己的 JSON.stringify 输出一致（见 order.go）：ZCode 读到
// 键序不同的文件会就地重写一遍，按它的顺序输出可以避免这种无谓的重写。
// 本包不做字节级保真：注释、空白与原始键序都会规范化。
func Encode(c *Config) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("provider_config.json 编码失败：Config 为 nil")
	}
	if c.doc == nil {
		return nil, fmt.Errorf("provider_config.json 编码失败：文档为空")
	}
	if err := Validate(c.doc); err != nil {
		return nil, err
	}
	data, err := marshalCanonical(c.doc)
	if err != nil {
		return nil, fmt.Errorf("provider_config.json 编码失败：%w", err)
	}
	return append(data, '\n'), nil
}
