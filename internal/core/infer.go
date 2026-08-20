package core

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var providerIDRe = regexp.MustCompile(`^[a-zA-Z0-9_:\-]+$`)

func ProviderIDValid(id string) bool {
	return providerIDRe.MatchString(strings.TrimSpace(id))
}

func GuessReasoning(modelID string) bool {
	low := strings.ToLower(modelID)
	for _, kw := range ReasoningKeywords {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

func AutoProviderID(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return ""
	}
	ipRe := regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`)
	if ipRe.MatchString(host) || host == "localhost" {
		pid := strings.ReplaceAll(host, ".", "-")
		return regexp.MustCompile(`[^a-zA-Z0-9_\-]`).ReplaceAllString(pid, "")
	}
	parts := strings.Split(host, ".")
	var pid string
	if len(parts) >= 2 {
		pid = parts[len(parts)-2]
	} else {
		pid = parts[0]
	}
	return regexp.MustCompile(`[^a-zA-Z0-9_\-]`).ReplaceAllString(pid, "")
}

func NewProviderID() string { return uuid.NewString() }

func InferKind(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "openai-compatible"
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "openai-compatible"
	}
	path := strings.ToLower(parsed.Path)
	if regexp.MustCompile(`(^|/)anthropic(/|$)`).MatchString(path) {
		return "anthropic"
	}
	return "openai-compatible"
}

func DefaultVariantsFor(kind string) []string {
	if v, ok := DefaultVariantsByKind[kind]; ok {
		out := make([]string, len(v))
		copy(out, v)
		return out
	}
	out := make([]string, len(DefaultVariantsByKind["openai-compatible"]))
	copy(out, DefaultVariantsByKind["openai-compatible"])
	return out
}

func DefaultVariantFor(kind string) string {
	if v, ok := DefaultVariantByKind[kind]; ok {
		return v
	}
	return DefaultVariantByKind["openai-compatible"]
}

func UIVariantOptions(kind string) []string {
	if v, ok := UIVariantOptionsByKind[kind]; ok {
		out := make([]string, len(v))
		copy(out, v)
		return out
	}
	out := make([]string, len(UIVariantOptionsByKind["openai-compatible"]))
	copy(out, UIVariantOptionsByKind["openai-compatible"])
	return out
}

func ValidateKind(kind string) (string, error) {
	if kind == "" {
		return "", &ConfigParseError{Msg: "协议类型（kind）只能是 openai-compatible、anthropic 或 responses"}
	}
	mapped, ok := ConfigKindToUI[kind]
	if ok {
		kind = mapped
	}
	for _, k := range Kinds {
		if k == kind {
			return kind, nil
		}
	}
	return "", &ConfigParseError{Msg: "协议类型（kind）只能是 openai-compatible、anthropic 或 responses"}
}

func ConfigKindToUIKind(kind, baseURL string) string {
	if kind == "" || strings.TrimSpace(kind) == "" {
		return InferKind(baseURL)
	}
	mapped, ok := ConfigKindToUI[strings.TrimSpace(kind)]
	if ok {
		for _, k := range Kinds {
			if k == mapped {
				return mapped
			}
		}
	}
	if kind == "openai" {
		return "responses"
	}
	return "openai-compatible"
}
