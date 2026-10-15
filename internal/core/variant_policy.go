package core

import "sync"

// customVariantsPolicy 守护“允许自定义推理档位”这个高级选项。默认关闭：
// 一切档位都必须落在 ZCodeVariantSet 白名单内；开启后放宽为「只去重 + 归一别名」。
//
// 并发安全照抄 urlutil.go 的 httpPolicy / privatePolicy：
// sync.RWMutex + 裸 bool，读写一律经 Set/Get 函数，绝不直接访问包级变量。
var (
	customVariantsMu sync.RWMutex
	customVariantsOn bool
)

// SetCustomVariantsAllowed 设置是否允许用户自填任意推理档位名（高级开关）。
// 关闭时（默认）一切档位都过 ZCodeVariantSet 白名单；打开时放宽为「只去重 + 归一别名」。
func SetCustomVariantsAllowed(v bool) {
	customVariantsMu.Lock()
	customVariantsOn = v
	customVariantsMu.Unlock()
}

// CustomVariantsAllowed 返回当前策略（供 core 内部与测试读取）。
func CustomVariantsAllowed() bool {
	customVariantsMu.RLock()
	defer customVariantsMu.RUnlock()
	return customVariantsOn
}
