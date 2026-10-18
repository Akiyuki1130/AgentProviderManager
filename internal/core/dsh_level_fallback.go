package core

import "sync"

// dshLevelFallbackPolicy 守护「DSH 写入路径用能力表兜底推理档位」这一策略（阶段 G）。
//
// 默认关闭：关闭时 DSH 的写盘结果与「能力表兜底」之前逐字节一致。开启后，只有
// 「卡片完全没有推理信息」的模型才会被补上一组推断档位——显式开/关与已有档位
// 一律不动（判定见 deepseek.go 的 dshFallbackEfforts）。
//
// 它跟着用户的「自动补全」开关走（app 层把 auto_fill_limits 推过来），因此 core 侧
// 只维护策略值，不读配置。
//
// 并发安全照抄 urlutil.go 的 httpPolicy / variant_policy.go：
// sync.RWMutex + 裸 bool，读写一律经 Set/Get 函数，绝不直接访问包级变量。
var (
	dshLevelFallbackMu sync.RWMutex
	dshLevelFallbackOn bool
)

// SetDshLevelFallbackAllowed 设置是否允许 DSH 写端用能力表兜底补推理档位。
func SetDshLevelFallbackAllowed(v bool) {
	dshLevelFallbackMu.Lock()
	dshLevelFallbackOn = v
	dshLevelFallbackMu.Unlock()
}

// DshLevelFallbackAllowed 返回当前策略（供 core 内部与测试读取）。
func DshLevelFallbackAllowed() bool {
	dshLevelFallbackMu.RLock()
	defer dshLevelFallbackMu.RUnlock()
	return dshLevelFallbackOn
}
