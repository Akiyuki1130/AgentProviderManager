package core

import (
	"reflect"
	"testing"
)

// map 迭代顺序是随机的：同一份配置重复读取必须得到同样的档位顺序，
// 否则 UI 每次刷新顺序都在变，保存时也会写出不同的 config。
func TestCfgToCardLegacyVariantsOrderIsDeterministic(t *testing.T) {
	cfg := map[string]interface{}{
		"reasoning": true,
		"variants": map[string]interface{}{
			"high":   map[string]interface{}{},
			"low":    map[string]interface{}{},
			"off":    map[string]interface{}{},
			"max":    map[string]interface{}{},
			"medium": map[string]interface{}{},
		},
	}
	want := []string{"off", "low", "medium", "high", "max"}
	for i := 0; i < 50; i++ {
		card := CfgToCard("m", cfg)
		if !reflect.DeepEqual(card.Variants, want) {
			t.Fatalf("第 %d 次读取：variants = %v，want %v", i, card.Variants, want)
		}
	}
}

func TestCfgToCardReasoningEffortsOrderIsDeterministic(t *testing.T) {
	cfg := map[string]interface{}{
		"reasoningEfforts": map[string]interface{}{
			"high": map[string]interface{}{"x": 1},
			"low":  map[string]interface{}{"x": 1},
			"off":  nil,
			"max":  map[string]interface{}{"x": 1},
		},
	}
	want := []string{"off", "low", "high", "max"}
	for i := 0; i < 50; i++ {
		card := CfgToCard("m", cfg)
		if !reflect.DeepEqual(card.Variants, want) {
			t.Fatalf("第 %d 次读取：variants = %v，want %v", i, card.Variants, want)
		}
	}
}
