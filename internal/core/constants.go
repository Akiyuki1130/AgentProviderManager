package core

const (
	AppName = "Agent Provider Manager"

	MaxModels          = 2000
	MaxModelIDLength   = 512
	MaxTokenLimit      = 10_000_000
	MaxBackups         = 2
	MaxProviderNameLen = 256
	MaxAPIKeyLength    = 8192
	MaxKeychainEntries = 500
	MaxKeychainNameLen = 256
	MaxKeychainNoteLen = 2048
	MaxKeychainIDLen   = 128
	KeychainFormatVer  = 2
	KeychainFilename   = "keychain.json"
	MaxOptionsLength   = 64 * 1024
	// MaxVariantLen 限自定义推理档位名的长度。ZCode/DSH 对档位名无 schema 校验，
	// 高级开关打开时用户可自填任意档位名，必须在收口处加帽，避免超长值落进配置。
	MaxVariantLen = 64
)

var Kinds = []string{"openai-compatible", "anthropic", "responses"}

// Config-layer kinds accepted by ZCode schema.
var ValidConfigKinds = map[string]bool{
	"openai-compatible": true,
	"anthropic":         true,
	"openai":            true,
}

var KindToConfig = map[string]string{
	"openai-compatible": "openai-compatible",
	"anthropic":         "anthropic",
	"responses":         "openai",
}

var ConfigKindToUI = map[string]string{
	"openai-compatible": "openai-compatible",
	"anthropic":         "anthropic",
	"openai":            "responses",
	"responses":         "responses",
}

var ReasoningKeywords = []string{
	"reason", "think", "thinker", "o1", "o3", "o4",
	"deepseek-r", "qwq", "thinking", "reasoning",
	"glm-zero", "kimi-think", "seed-think",
}

// ReasoningVariantCore 是规范档位全集（off 起、max 止），同时也是 DSH 的固定 7 档顺序。
//
// 交叉指向：deepseek.go 的 dshReasoningEffortOrder 与本切片完全同序。按本阶段的文件
// 归属约定不修改 deepseek.go，因此未把它改成对本切片的引用——二者若有一处调整，另一处
// 必须同步（见 M5 清理说明）。
var ReasoningVariantCore = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// ZCodeVariantOptions 是 ZCode 侧可识别的档位词全集。
//
// 顺序约定：先是规范档位全集（ReasoningVariantCore：off,minimal,low,medium,high,xhigh,max），
// 随后是 ZCode 特有的单元素语义 "enabled"，最后是仅用于“识别别名”的历史拼法
// （disabled/none/nothink）——它们经 canonicalVariant 会折成 "off"。
// variantOrderIndex 依赖这个顺序给档位排序，因此规范档位必须排在别名之前。
var ZCodeVariantOptions = append(append([]string{}, ReasoningVariantCore...),
	"enabled",
	"disabled", "none", "nothink",
)

var ZCodeVariantSet = func() map[string]bool {
	m := make(map[string]bool, len(ZCodeVariantOptions))
	for _, v := range ZCodeVariantOptions {
		m[v] = true
	}
	return m
}()

// UIVariantOptionsByKind 是编辑器“可选档位”的全集（按协议类型给出，当前三种一致）。
// 阶段 A 起改为反映规范档位全集（含 minimal）；默认勾选集合仍是 {off,high,max}，
// 见 DefaultVariantsByKind。
var UIVariantOptionsByKind = map[string][]string{
	"openai-compatible": {"off", "minimal", "low", "medium", "high", "xhigh", "max"},
	"anthropic":         {"off", "minimal", "low", "medium", "high", "xhigh", "max"},
	"responses":         {"off", "minimal", "low", "medium", "high", "xhigh", "max"},
}

// DefaultVariantsByKind 是新建 / 开启思考时的默认勾选集合，语义保持不变（{off,high,max}）。
var DefaultVariantsByKind = map[string][]string{
	"openai-compatible": {"off", "high", "max"},
	"anthropic":         {"off", "high", "max"},
	"responses":         {"off", "high", "max"},
}

var DefaultVariantByKind = map[string]string{
	"openai-compatible": "high",
	"anthropic":         "high",
	"responses":         "high",
}

// DefaultVariantPreference 是“为档位集合挑默认档位”时的优先级顺序（high 优先）。
//
// 与前端 pickDefaultVariant 及 DefaultVariantByKind 三者必须一致（M1：统一为 high 优先）。
// 原先以 "max" 开头属既有不一致，本阶段改正。
//
// 说明：
//   - 已删除死项 "none"——它经 canonicalVariant 恒折成 "off"，绝不会以 "none" 形态出现；
//   - "enabled" 是 ZCode 特有单元素语义，排在真实强度档位之后作为兜底。
var DefaultVariantPreference = []string{
	"high", "max", "xhigh", "medium", "low",
	"minimal", "enabled", "off",
}
