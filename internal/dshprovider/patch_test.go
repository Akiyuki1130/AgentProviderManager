package dshprovider

import (
	"path/filepath"
	"strings"
	"testing"
)

const jsPatch = `# Your patch layer for this dsh profile, applied after every bundle layer:
# a top-level YAML array of loader patch entries (id-targeted config
# overrides, disables, and insert lists; !!js expressions allowed).
- id: hmr
  config:
    root: []
- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        displayName: Acme
        apiKeyEnv: ACME_API_KEY
        api: openai-completions
        baseURL: https://acme.example/v1
        models:
          - id: acme-think
            name: Acme Think
            contextWindow: 262144
- id: tools
  config:
    mode: !!js process.env.DSH_TOOLS_MODE
`

func TestLoadPatchKeepsJSTagsAndComments(t *testing.T) {
	p, err := LoadPatch([]byte(jsPatch))
	if err != nil {
		t.Fatalf("LoadPatch: %v", err)
	}
	if p.Len() != 3 {
		t.Fatalf("Len = %d, want 3", p.Len())
	}
	if !p.HasLLMPiAIEntry() {
		t.Fatal("HasLLMPiAIEntry = false, want true")
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "!!js process.env.DSH_TOOLS_MODE") {
		t.Fatalf("!!js expression lost: %s", text)
	}
	if !strings.Contains(text, "# Your patch layer for this dsh profile") {
		t.Fatalf("comment lost: %s", text)
	}
	if !strings.Contains(text, "root: []") {
		t.Fatalf("untouched row lost: %s", text)
	}
}

func TestSetProvidersRoundTrip(t *testing.T) {
	p, err := LoadPatch([]byte(jsPatch))
	if err != nil {
		t.Fatalf("LoadPatch: %v", err)
	}
	providers, err := p.Providers()
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("providers = %d, want 1", len(providers))
	}
	providers["second"] = map[string]interface{}{
		"displayName": "Second",
		"apiKeyEnv":   "SECOND_API_KEY",
		"api":         "anthropic-messages",
		"baseURL":     "https://second.example",
		"models":      []interface{}{map[string]interface{}{"id": "second-1"}},
	}
	if err := p.SetProviders(providers); err != nil {
		t.Fatalf("SetProviders: %v", err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	reloaded, err := LoadPatch(out)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	back, err := reloaded.Providers()
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	if len(back) != 2 {
		t.Fatalf("providers after round trip = %d, want 2: %v", len(back), back)
	}
	second, _ := back["second"].(map[string]interface{})
	if second == nil || second["api"] != "anthropic-messages" {
		t.Fatalf("second provider not preserved: %v", back["second"])
	}
	if reloaded.Len() != 3 {
		t.Fatalf("row count changed: %d", reloaded.Len())
	}
}

func TestSetProvidersCreatesRowWhenAbsent(t *testing.T) {
	p, err := LoadPatch([]byte("[]"))
	if err != nil {
		t.Fatalf("LoadPatch: %v", err)
	}
	if p.HasLLMPiAIEntry() {
		t.Fatal("empty patch should not have the row")
	}
	if err := p.SetProviders(map[string]interface{}{"only": map[string]interface{}{"baseURL": "https://x.example"}}); err != nil {
		t.Fatalf("SetProviders: %v", err)
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.Contains(string(out), "id: llm-pi-ai") {
		t.Fatalf("row id missing: %s", out)
	}
	if !strings.Contains(string(out), "@deepseek-ai/dsh-llm-pi-ai") {
		t.Fatalf("plugin name missing: %s", out)
	}
}

func TestCommentOnlyPatchIsEmpty(t *testing.T) {
	p, err := LoadPatch([]byte("# only a comment"))
	if err != nil {
		t.Fatalf("LoadPatch: %v", err)
	}
	if p.Len() != 0 {
		t.Fatalf("Len = %d, want 0", p.Len())
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if string(out) != "[]" + "\n" {
		t.Fatalf("empty patch must serialize as [] to keep DSH bootable, got %q", out)
	}
}

func TestPatchRootMustBeSequence(t *testing.T) {
	if _, err := LoadPatch([]byte("id: llm-pi-ai")); err == nil {
		t.Fatal("expected an error for a mapping root")
	}
}

func TestDefinesLLMPiAIProviders(t *testing.T) {
	yes, err := DefinesLLMPiAIProviders([]byte(jsPatch))
	if err != nil || !yes {
		t.Fatalf("want true, got %v (%v)", yes, err)
	}
	no, err := DefinesLLMPiAIProviders([]byte("- id: hmr"))
	if err != nil || no {
		t.Fatalf("want false, got %v (%v)", no, err)
	}
}

func TestDetectFormat(t *testing.T) {
	home := filepath.Join("C:", "Users", "placeholder", ".dsh")
	cases := []struct {
		path string
		data string
		want Format
	}{
		{ProfilePatchPath(home, DesktopProfileName), "[]", FormatProfileDesktop},
		{ProfilePatchPath(home, WebProfileName), "[]", FormatProfileWeb},
		{ProfilePatchPath(home, "acme"), "[]", FormatProfileOther},
		{filepath.Join(home, "settings.yaml"), "llm-pi-ai:" + "\n" + "  providers: {}", FormatSettingsLegacy},
		{filepath.Join(home, "settings.json"), "{}", FormatSettingsLegacy},
		{filepath.Join(home, "unknown.json"), "{\"provider\":{}}", FormatUnknown},
		{filepath.Join(home, "custom.yml"), jsPatch, FormatProfileOther},
	}
	for _, c := range cases {
		if got := Detect(c.path, []byte(c.data)); got != c.want {
			t.Errorf("Detect(%s) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestProfileNameAt(t *testing.T) {
	home := filepath.Join("C:", "Users", "placeholder", ".dsh")
	if got := ProfileNameAt(ProfilePatchPath(home, DesktopProfileName)); got != "desktop" {
		t.Fatalf("ProfileNameAt = %q", got)
	}
	if got := ProfileNameAt(filepath.Join(home, "cordis.patch.yml")); got != "" {
		t.Fatalf("home patch must not resolve to a profile name, got %q", got)
	}
	if got := ProfileNameAt(filepath.Join(home, "some", "other", "cordis.patch.yml")); got != "" {
		t.Fatalf("non-profile patch must not resolve, got %q", got)
	}
}
