package core

import (
	"encoding/json"
	"testing"
)

// assertNotNullJSONField 断言 v 的某个数组字段编码成 []，而不是 null。
// Go 的 encoding/json 把 nil slice 编成 null，而 ProviderEdit.cards、ProviderSummary
// 列表与 ModelCard.variants 都没有 omitempty：读取路径返回 nil 时，前端拿到的是 null
// 而不是数组，会在 cards.map / variants.map 上抛 “Cannot read properties of null”。
func assertNotNullJSONField(t *testing.T, label string, v interface{}, field string) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s：json.Marshal 失败：%v", label, err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("%s：json.Unmarshal 失败：%v", label, err)
	}
	raw, ok := obj[field]
	if !ok {
		t.Fatalf("%s：JSON 里缺少字段 %q：%s", label, field, data)
	}
	if raw == nil {
		t.Errorf("%s：字段 %q 序列化为 null（应为 []）：%s", label, field, data)
	}
}

// TestProviderToEditZeroModelCardsNotNull 锁住编辑器读取路径：
// provider 一条模型都没有时，ProviderEdit.cards 必须是空数组而不是 nil。
// 零模型 provider 是用户可保存的正常状态（先建 provider 后加模型，或删光模型），
// 此时前端 loadProvider 会在 cards.map 上抛错，整个编辑页打不开。
func TestProviderToEditZeroModelCardsNotNull(t *testing.T) {
	cases := []struct {
		label    string
		provider map[string]interface{}
	}{
		{"没有 models 键", map[string]interface{}{
			"name": "No Models", "kind": "openai-compatible",
			"options": map[string]interface{}{"baseURL": "https://nomodels.example.com/v1"},
		}},
		{"models 是空对象", map[string]interface{}{
			"name": "No Models", "kind": "openai-compatible",
			"models": map[string]interface{}{},
		}},
	}
	for _, tc := range cases {
		cfg := map[string]interface{}{"provider": map[string]interface{}{"nomodels": tc.provider}}
		edit, err := ProviderToEdit(cfg, "nomodels")
		if err != nil {
			t.Fatalf("%s：ProviderToEdit 失败：%v", tc.label, err)
		}
		if edit.Cards == nil {
			t.Errorf("%s：Cards 不应是 nil", tc.label)
		}
		if len(edit.Cards) != 0 {
			t.Fatalf("%s：Cards 应为空，得到 %+v", tc.label, edit.Cards)
		}
		assertNotNullJSONField(t, tc.label, edit, "cards")
	}

	// 非空情形不受影响。
	cfg := map[string]interface{}{"provider": map[string]interface{}{"one": map[string]interface{}{
		"name":   "One",
		"models": map[string]interface{}{"m1": map[string]interface{}{"name": "m1"}},
	}}}
	edit, err := ProviderToEdit(cfg, "one")
	if err != nil {
		t.Fatalf("ProviderToEdit(one) 失败：%v", err)
	}
	if len(edit.Cards) != 1 {
		t.Fatalf("应得到 1 张卡，得到 %+v", edit.Cards)
	}
}

// TestBuildProviderSummaryEmptyConfigNotNil 锁住提供商列表：
// 配置文件里连 provider 键都没有（新装用户）时也必须给 [] 而不是 null。
func TestBuildProviderSummaryEmptyConfigNotNil(t *testing.T) {
	for _, cfg := range []map[string]interface{}{
		nil,
		{},
		{"provider": map[string]interface{}{}},
	} {
		summaries := BuildProviderSummary(cfg)
		if summaries == nil {
			t.Errorf("BuildProviderSummary(%#v) 不应返回 nil", cfg)
			continue
		}
		data, err := json.Marshal(summaries)
		if err != nil {
			t.Fatalf("BuildProviderSummary(%#v)：json.Marshal 失败：%v", cfg, err)
		}
		if string(data) != "[]" {
			t.Errorf("BuildProviderSummary(%#v) 应序列化为 []，得到 %s", cfg, data)
		}
	}
}

// TestCfgToCardVariantsNotNull 锁住同一个契约在模型卡上的孪生位置：
// NormalizeVariants 对 nil / 全被过滤掉的输入返回 nil，CfgToCard 不能把它直接
// 赋给没有 omitempty 的 variants 字段。取值语义保持不变。
func TestCfgToCardVariantsNotNull(t *testing.T) {
	cases := []struct {
		label    string
		modelCfg map[string]interface{}
	}{
		{"没有 reasoning 键", map[string]interface{}{"name": "m"}},
		{"reasoning 存在但没有 variants", map[string]interface{}{"reasoning": map[string]interface{}{"enabled": true}}},
		{"variants 全被过滤掉", map[string]interface{}{"reasoning": map[string]interface{}{"variants": []interface{}{"not-a-variant"}}}},
		{"reasoning 是布尔", map[string]interface{}{"reasoning": false}},
	}
	for _, tc := range cases {
		card := CfgToCard("m", tc.modelCfg)
		if card.Variants == nil {
			t.Errorf("%s：Variants 不应是 nil", tc.label)
		}
		if len(card.Variants) != 0 {
			t.Errorf("%s：Variants 应为空，得到 %v", tc.label, card.Variants)
		}
		assertNotNullJSONField(t, tc.label, card, "variants")
	}

	card := CfgToCard("m", map[string]interface{}{
		"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"off", "high"}},
	})
	if len(card.Variants) != 2 || card.Variants[0] != "off" || card.Variants[1] != "high" {
		t.Errorf("variants 应保持 off,high，得到 %v", card.Variants)
	}
	if !card.Reasoning {
		t.Error("off+high 档位时 Reasoning 应为 true")
	}
	card = CfgToCard("m", map[string]interface{}{
		"reasoning": map[string]interface{}{"variants": []interface{}{"off"}},
	})
	if len(card.Variants) != 1 || card.Variants[0] != "off" {
		t.Errorf("variants 应为 off，得到 %v", card.Variants)
	}
	if card.Reasoning {
		t.Error("只有 off 档位时 Reasoning 应为 false")
	}
}
