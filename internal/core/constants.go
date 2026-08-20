package core

const (
	AppName    = "Agent Provider Manager"
	AppVersion = "1.0.1"

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

var ZCodeVariantOptions = []string{
	"off", "enabled", "disabled", "none", "nothink", "minimal",
	"low", "medium", "high", "xhigh", "max",
}

var ZCodeVariantSet = func() map[string]bool {
	m := make(map[string]bool, len(ZCodeVariantOptions))
	for _, v := range ZCodeVariantOptions {
		m[v] = true
	}
	return m
}()

var UIVariantOptionsByKind = map[string][]string{
	"openai-compatible": {"off", "low", "medium", "high", "xhigh", "max"},
	"anthropic":         {"off", "low", "medium", "high", "xhigh", "max"},
	"responses":         {"off", "low", "medium", "high", "xhigh", "max"},
}

var DefaultVariantsByKind = map[string][]string{
	"openai-compatible": {"off", "high", "max"},
	"anthropic":         {"off", "high", "max"},
	"responses":         {"off", "high", "max"},
}

var DefaultVariantByKind = map[string]string{
	"openai-compatible": "max",
	"anthropic":         "max",
	"responses":         "max",
}

var OpencodeEffortToVariant = map[string]string{
	"none":     "off",
	"minimal":  "low",
	"disabled": "off",
	"nothink":  "off",
}

var OpencodeEffortOrder = []string{"low", "medium", "high", "xhigh", "max"}

var DefaultVariantPreference = []string{
	"max", "high", "xhigh", "medium", "low",
	"minimal", "enabled", "off", "none",
}
