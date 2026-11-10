package core

import (
	"reflect"
	"testing"
)

// 本文件锁住 TODO #28 与 #30② 的 DSH 写端约束（决定见 lead 裁定，理由随测试注释保留）：
//   - #28：只含 off 的源档位集合 → 一个推理键都不写（不写 reasoningEfforts:false）；
//   - #28：显式关闭（RawReasoningEnabled=false）→ 仍写 reasoningEfforts:false；
//   - #30②：写端永远只写固定 7 档，自填档位名不落盘（schema 边界）；
//   - #30①：modelOverrides 里每个模型的 compat 按 wire protocol 过滤，复用 dshCompatForWrite。
//
// 全部是纯内存用例，不触盘、只用占位值。

// TestDshExplicitReasoningFalsePreserved 是 #28 的反向锁定：删掉「只含 off → false」的
// 退化分支，不得误伤「用户显式关闭思考」这条真实语义——它必须原样写 false。
func TestDshExplicitReasoningFalsePreserved(t *testing.T) {
	off := false
	node := dshModelToRaw(ModelCard{
		ModelID: "demo", Name: "demo", Reasoning: true, RawReasoningEnabled: &off,
		Variants: []string{"off", "high"}, RawCfg: map[string]interface{}{},
	}, "", dshTestAPI)
	if node["reasoningEfforts"] != false {
		t.Fatalf("显式关闭仍必须写 reasoningEfforts:false，实际 %#v", node["reasoningEfforts"])
	}
	if _, has := node["reasoningEffort"]; has {
		t.Fatalf("显式关闭后不得残留 reasoningEffort：%#v", node["reasoningEffort"])
	}
}

// TestDshEffortForWriteOnlySevenLevels 直接锁住 #30② 的门控粒度：写端只认 dshCanonicalEffort
// 的 7 档（含别名归一），其余一律不落盘；读端在开关打开时仍保留自填名。
func TestDshEffortForWriteOnlySevenLevels(t *testing.T) {
	setCustomVariantsForTest(t, true)

	for _, name := range dshAllEfforts {
		if got, ok := dshEffortForWrite(name); !ok || got != name {
			t.Fatalf("七档 %q 应原样可写，实际 (%q, %v)", name, got, ok)
		}
	}
	if got, ok := dshEffortForWrite("disabled"); !ok || got != "off" {
		t.Fatalf("off 别名应归一为 off，实际 (%q, %v)", got, ok)
	}
	for _, custom := range []string{"very-high", "ultra", "超高"} {
		if got, ok := dshEffortForWrite(custom); ok {
			t.Fatalf("自填档位 %q 不得写盘，实际写出 %q（会让整份 llm-pi-ai 解析失败）", custom, got)
		}
	}
	// 读端刻意与写端不对称：写不落盘只是 DSH 的 schema 边界，卡片数据不该丢。
	if got, ok := dshEffortForRead("very-high"); !ok || got != "very-high" {
		t.Fatalf("读端应保留自填档位，实际 (%q, %v)", got, ok)
	}
	// 开关关闭时读端仍按白名单丢弃（默认行为不变）。
	setCustomVariantsForTest(t, false)
	if got, ok := dshEffortForRead("very-high"); ok {
		t.Fatalf("开关关闭时读端不得保留自填档位，实际 %q", got)
	}
}

// TestDshModelOverridesCompatFilteredByProtocol 锁住 #30①：modelOverrides 里每个模型的
// compat 必须按该 provider 的 wire protocol 过滤，且复用 dshCompatForWrite（与模型节点、
// 路由级同一实现）。错位的 compat 键会被 DSH 判为「which does not take it」，在严格解析下
// 让整份 llm-pi-ai 配置抛错——所以透传前必须剥掉。
func TestDshModelOverridesCompatFilteredByProtocol(t *testing.T) {
	opts := map[string]interface{}{
		"modelOverrides": map[string]interface{}{
			"demo": map[string]interface{}{
				"defaultContextWindow": 111,
				"compat": map[string]interface{}{
					"supportsTemperature": true, // anthropic 接受，completions 不接受
					"supportsStrictMode":  true, // completions 接受，anthropic 不接受
					"supportsToolSearch":  true, // 所有协议都不认
				},
			},
			"plain-string": "not-a-map",
		},
		"timeoutMs": 1000,
	}

	cases := []struct {
		name       string
		api        string
		wantCompat map[string]interface{}
	}{
		{"anthropic 只留协议内键", "anthropic-messages", map[string]interface{}{"supportsTemperature": true}},
		{"completions 只留协议内键", "openai-completions", map[string]interface{}{"supportsStrictMode": true}},
		{"未知协议 fail-closed 全剥", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			extras := deepSeekRouteExtras(opts, tc.api)
			overrides, ok := extras["modelOverrides"].(map[string]interface{})
			if !ok {
				t.Fatalf("modelOverrides 未透传：%#v", extras)
			}
			demo, ok := overrides["demo"].(map[string]interface{})
			if !ok {
				t.Fatalf("demo 覆盖项缺失：%#v", overrides)
			}
			got, has := demo["compat"]
			if tc.wantCompat == nil {
				if has {
					t.Fatalf("协议不认的 compat 应整键省略，实际 %#v", got)
				}
			} else if !reflect.DeepEqual(got, tc.wantCompat) {
				t.Fatalf("compat = %#v，期望 %#v", got, tc.wantCompat)
			}
			// deepCopyMap 走 JSON 往返，数值统一是 float64——这里只断言键没被过滤掉。
			if got, ok := demo["defaultContextWindow"].(float64); !ok || got != 111 {
				t.Fatalf("非 compat 的覆盖键必须原样透传：%#v", demo)
			}
			if overrides["plain-string"] != "not-a-map" {
				t.Fatalf("非映射形状的覆盖项应原样透传：%#v", overrides)
			}
			if extras["timeoutMs"] != 1000 {
				t.Fatalf("其它路由字段应原样透传：%#v", extras)
			}
		})
	}

	// 过滤不得改动调用方传入的 options（deepCopyMap 隔离），否则编辑器再次保存时会
	// 拿着「已被剥过」的覆盖项，读盘侧就丢了用户原始配置。
	srcCompat, _ := opts["modelOverrides"].(map[string]interface{})["demo"].(map[string]interface{})["compat"].(map[string]interface{})
	if len(srcCompat) != 3 {
		t.Fatalf("过滤不得就地修改入参 compat，实际 %#v", srcCompat)
	}
}
