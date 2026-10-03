package zcodeprovider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// ZCode 自己的写法是 JSON.stringify(doc, null, 2)，并且读取时会比较
// “磁盘上的字节解析结果”与“重新编码后的结果”：只要键序不同，它就会把文件重写一遍。
// 为了不产生这种无谓的重写（也让指纹稳定），本包按 ZCode 的键序输出。
//
// 顺序表全部来自 ZCode 3.14 的解码/编码代码：
//   - 根与 config：encodeProviderConfigFile 的构造顺序；
//   - providerConfigRules / modelConfigRules：各自的 zod 对象键序；
//   - providerRule / providerRuleConfig / access / api / modelRule / modelRuleConfig：
//     对应类的序列化顺序。
//
// 本包没有建模的合法键会被追加在已知键之后（按字母序），保证既不丢字段也保持稳定。

var rootOrder = []string{"schemaVersion", "config"}

var configOrder = []string{
	"providerOrder",
	"providerConfigRules",
	"modelConfigRules",
	"defaultModelSelection",
}

var providerConfigRulesOrder = []string{"providerRules"}

var modelConfigRulesOrder = []string{"providerModelRules", "manualProviderModelRules"}

var providerRuleOrder = []string{"providerId", "providerName", "templateId", "enabled", "config"}

var providerRuleConfigOrder = []string{
	"group",
	"logo",
	"access",
	"api",
	"builtinModelIds",
	"personalModelIds",
	"modelOrder",
	"visibility",
}

var accessOrder = []string{"type", "apiKey", "apiKeyManagementUrl", "accountType", "mode", "entitled"}

var apiOrder = []string{"type", "baseUrl", "headers"}

var modelRuleOrder = []string{"providerId", "modelId", "config"}

var modelRuleConfigOrder = []string{"enabled", "properties", "optionSpecs"}

var propertiesOrder = []string{
	"requiresMfjsToolSchema",
	"contextWindow",
	"inputFormat",
	"outputFormat",
	"supportsToolCall",
	"supportsJsonSchemaOutput",
	"supportsNativeWebSearch",
	"supportsMidConversationSystem",
}

var inputFormatOrder = []string{"supportsText", "supportsImage", "supportsVideo", "supportsAudio", "supportsPdf"}

var outputFormatOrder = []string{"supportsText"}

var optionSpecsOrder = []string{"reasoningLevel", "maxOutputTokens"}

var valuesMapOrder = []string{"values", "map"}

var maxMapOrder = []string{"max", "map"}

// orderFor 返回某个键对应的取值应该使用的键顺序。
// key 是它在父对象里的键名；数组元素的顺序由数组键名 + 元素内容推断。
func orderFor(key string, v interface{}) []string {
	obj, isObj := v.(map[string]interface{})
	if !isObj {
		return nil
	}
	switch key {
	case "":
		return rootOrder
	case "config":
		switch {
		case hasKey(obj, "providerConfigRules"), hasKey(obj, "modelConfigRules"):
			return configOrder
		case hasKey(obj, "enabled"), hasKey(obj, "properties"), hasKey(obj, "optionSpecs"):
			return modelRuleConfigOrder
		default:
			return providerRuleConfigOrder
		}
	case "providerConfigRules":
		return providerConfigRulesOrder
	case "modelConfigRules":
		return modelConfigRulesOrder
	case "providerRules":
		return providerRuleOrder
	case "providerModelRules", "manualProviderModelRules":
		return modelRuleOrder
	case "access":
		return accessOrder
	case "api":
		return apiOrder
	case "headers":
		return nil // 用户自定义头：按字母序
	case "logo":
		return []string{"type", "key"}
	case "properties":
		return propertiesOrder
	case "inputFormat":
		return inputFormatOrder
	case "outputFormat":
		return outputFormatOrder
	case "optionSpecs":
		return optionSpecsOrder
	case "reasoningLevel":
		return valuesMapOrder
	case "maxOutputTokens":
		return maxMapOrder
	case "defaultModelSelection":
		return nil
	default:
		// providerRule / modelRule 这类数组元素：键名沿用数组名，元素本身是规则对象。
		return nil
	}
}

func hasKey(obj map[string]interface{}, key string) bool {
	_, ok := obj[key]
	return ok
}

// marshalCanonical 按键序输出 JSON（2 空格缩进，无末尾换行）。
func marshalCanonical(doc map[string]interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeCanonical(&buf, doc, 0, ""); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v interface{}, level int, key string) error {
	switch value := v.(type) {
	case map[string]interface{}:
		order := orderFor(key, value)
		keys := orderedKeys(value, order)
		if len(keys) == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				buf.WriteString(",")
			}
			buf.WriteByte('\n')
			writeIndent(buf, level+1)
			if err := writeJSONString(buf, k); err != nil {
				return err
			}
			buf.WriteString(": ")
			if err := writeCanonical(buf, value[k], level+1, k); err != nil {
				return err
			}
		}
		buf.WriteByte('\n')
		writeIndent(buf, level)
		buf.WriteString("}")
		return nil
	case []interface{}:
		if len(value) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteString("[")
		for i, item := range value {
			if i > 0 {
				buf.WriteString(",")
			}
			buf.WriteByte('\n')
			writeIndent(buf, level+1)
			// 数组元素的顺序由元素自身的形状决定（规则对象用规则键序）。
			elemKey := elementKey(key, item)
			if err := writeCanonical(buf, item, level+1, elemKey); err != nil {
				return err
			}
		}
		buf.WriteByte('\n')
		writeIndent(buf, level)
		buf.WriteString("]")
		return nil
	case nil:
		buf.WriteString("null")
		return nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		buf.Write(data)
		return nil
	}
}

// elementKey 把数组元素的键名映射成它所适用的顺序表键名。
func elementKey(arrayKey string, item interface{}) string {
	if arrayKey == "providerRules" || arrayKey == "providerModelRules" || arrayKey == "manualProviderModelRules" {
		return arrayKey
	}
	obj, ok := item.(map[string]interface{})
	if !ok {
		return arrayKey
	}
	switch {
	case hasKey(obj, "providerId"), hasKey(obj, "modelId"), hasKey(obj, "config"):
		if hasKey(obj, "modelId") {
			return "providerModelRules"
		}
		return "providerRules"
	default:
		return arrayKey
	}
}

// orderedKeys 先按 order 排列已知键，再把其余键按字母序追加。
func orderedKeys(obj map[string]interface{}, order []string) []string {
	keys := make([]string, 0, len(obj))
	seen := make(map[string]bool, len(obj))
	for _, k := range order {
		if _, ok := obj[k]; !ok || seen[k] {
			continue
		}
		keys = append(keys, k)
		seen[k] = true
	}
	rest := make([]string, 0, len(obj))
	for k := range obj {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func writeIndent(buf *bytes.Buffer, level int) {
	for i := 0; i < level; i++ {
		buf.WriteString("  ")
	}
}

// writeJSONString 按 encoding/json 的规则输出字符串（含转义）。
func writeJSONString(buf *bytes.Buffer, s string) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("编码键名失败：%w", err)
	}
	buf.Write(data)
	return nil
}
