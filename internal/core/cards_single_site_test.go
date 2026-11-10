package core

import (
	"reflect"
	"testing"
)

// TestBuildSingleCardForSite 锁定「手动添加模型」与拉取路径的口径一致性（TODO 第 29 项）：
//  1. 没有站点上下文时与 BuildSingleCard 逐字段相同（不改变既有调用方行为）。
//  2. 站点上下文参与后，结果与 BuildModelCardsForSite 对同一模型/站点给出的卡片一致。
//  3. 空模型 ID 仍然报错。
func TestBuildSingleCardForSite(t *testing.T) {
	t.Run("无站点上下文等价于 BuildSingleCard", func(t *testing.T) {
		base, err := BuildSingleCard("glm-5.3", true)
		if err != nil {
			t.Fatalf("BuildSingleCard 报错：%v", err)
		}
		site, err := BuildSingleCardForSite("glm-5.3", true, "", "")
		if err != nil {
			t.Fatalf("BuildSingleCardForSite 报错：%v", err)
		}
		if !reflect.DeepEqual(base, site) {
			t.Fatalf("空站点上下文下两函数结果不一致：\nbase=%+v\nsite=%+v", base, site)
		}
	})

	t.Run("与 BuildModelCardsForSite 同口径", func(t *testing.T) {
		const (
			modelID = "glm-5.3"
			baseURL = "https://api.z.ai/api/anthropic"
			apiType = "anthropic-messages"
		)
		want := BuildModelCardsForSite([]struct {
			ID   string
			Meta map[string]interface{}
		}{{ID: modelID, Meta: map[string]interface{}{}}}, true, baseURL, apiType)[0]
		got, err := BuildSingleCardForSite(modelID, true, baseURL, apiType)
		if err != nil {
			t.Fatalf("BuildSingleCardForSite 报错：%v", err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("与 BuildModelCardsForSite 口径不一致：\nwant=%+v\ngot=%+v", want, got)
		}
	})

	t.Run("空模型 ID 报错", func(t *testing.T) {
		if _, err := BuildSingleCardForSite("   ", true, "", ""); err == nil {
			t.Fatal("空模型 ID 应当报错")
		}
	})
}
