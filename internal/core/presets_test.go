package core

import "testing"

func TestLookupModelLimitsExact(t *testing.T) {
	cases := []struct {
		id      string
		wantCtx int
		wantOut int
	}{
		{"deepseek-v4-flash", 1000000, 384000},
		{"glm-5.2", 1000000, 128000},
		{"glm-5.3", 1000000, 128000},
		{"gpt-5.6-luna", 1050000, 128000},
		{"gpt-5.6-terra", 1050000, 128000},
		{"gpt-5.6-sol", 1050000, 128000},
		{"gemini-3.7-flash", 1048576, 65536},
		{"qwen3.7-max", 1000000, 131000},
		{"qwen3.8-max", 1000000, 131000},
		{"claude-opus-5", 1000000, 128000},
		{"claude-haiku-4-5", 200000, 64000},
		{"kimi-k3", 1048576, 128000},
		{"hunyuan-a13b", 224000, 32000},
		{"doubao-seed-evolving", 1024000, 256000},
		{"mimo-v2.5-pro", 1048576, 128000},
		{"mimo-v2.5", 1048576, 128000},
		{"mimo-v2-flash", 262144, 65536},
	}
	for _, c := range cases {
		p, ok := LookupModelLimits(c.id)
		if !ok {
			t.Errorf("%s: not found", c.id)
			continue
		}
		if p.Context != c.wantCtx || p.Output != c.wantOut {
			t.Errorf("%s: got ctx=%d out=%d, want ctx=%d out=%d", c.id, p.Context, p.Output, c.wantCtx, c.wantOut)
		}
	}
}

func TestLookupModelLimitsCaseInsensitive(t *testing.T) {
	p, ok := LookupModelLimits("  GPT-5.6-LUNA ")
	if !ok || p.Context != 1050000 || p.Output != 128000 {
		t.Fatalf("case-insensitive match failed: %+v ok=%v", p, ok)
	}
}

func TestLookupModelLimitsSnapshotPrefix(t *testing.T) {
	cases := []struct {
		id      string
		wantCtx int
		wantOut int
	}{
		{"deepseek-v4-flash-0731", 1000000, 384000},
		{"deepseek-v4-pro-0813", 1000000, 384000},
		{"gpt-5.6-luna-2026-08-01", 1050000, 128000},
		{"claude-opus-4-6-2026-01-20", 1000000, 128000},
		{"claude-sonnet-4-6-2025-10-14", 1000000, 128000},
		{"qwen3.7-plus-2026-05-26", 1000000, 131000},
		{"gemini-3.7-flash-001", 1048576, 65536},
		{"glm-4.7-flash-x-2026-01-01", 200000, 128000},
	}
	for _, c := range cases {
		p, ok := LookupModelLimits(c.id)
		if !ok {
			t.Errorf("%s: not found", c.id)
			continue
		}
		if p.Context != c.wantCtx || p.Output != c.wantOut {
			t.Errorf("%s: got ctx=%d out=%d, want ctx=%d out=%d", c.id, p.Context, p.Output, c.wantCtx, c.wantOut)
		}
	}
}

func TestLookupModelLimitsLongestPrefixWins(t *testing.T) {
	// claude-opus-4 is a prefix of claude-opus-4-6; the longer key must win.
	p, ok := LookupModelLimits("claude-opus-4-6-2026-01-20")
	if !ok || p.Context != 1000000 {
		t.Fatalf("longest prefix not used: %+v ok=%v", p, ok)
	}
	// bare claude-opus-4 still resolves to its own entry.
	p, ok = LookupModelLimits("claude-opus-4")
	if !ok || p.Context != 200000 || p.Output != 32000 {
		t.Fatalf("bare claude-opus-4: %+v ok=%v", p, ok)
	}
}

func TestLookupModelLimitsUnknown(t *testing.T) {
	if _, ok := LookupModelLimits("foo-model-xyz"); ok {
		t.Fatal("unknown model should not match")
	}
	if _, ok := LookupModelLimits(""); ok {
		t.Fatal("empty model should not match")
	}
}

func TestLookupModelLimitsContextOnly(t *testing.T) {
	for _, id := range []string{"grok-4.6", "grok-4.3", "llama-4-maverick", "minimax-m3", "qwen-long", "moonshot-v1-128k"} {
		p, ok := LookupModelLimits(id)
		if !ok {
			t.Errorf("%s: not found", id)
			continue
		}
		if p.Context <= 0 {
			t.Errorf("%s: context should be set", id)
		}
	}
	// Output side may be zero for these families (not documented).
	if p, _ := LookupModelLimits("grok-4.6"); p.Output != 0 {
		t.Errorf("grok-4.6 output should be 0 (not documented), got %d", p.Output)
	}
}

func TestApplyModelPresets(t *testing.T) {
	cards := []ModelCard{
		{ModelID: "deepseek-v4-flash", Context: "", Output: ""},
		{ModelID: "gpt-5.6-luna", Context: nil, Output: nil},
		{ModelID: "claude-opus-4-6", Context: 900000, Output: "64k"}, // preset overwrites filled values
		{ModelID: "grok-4.6", Context: "", Output: ""},               // context only (output not in preset)
		{ModelID: "mystery-model", Context: "64k", Output: "8k"},     // no preset, must stay untouched
	}
	updated, filled := ApplyModelPresets(cards)
	if filled != 7 {
		t.Fatalf("filled=%d, want 7", filled)
	}
	if updated[0].Context != 1000000 || updated[0].Output != 384000 {
		t.Errorf("deepseek-v4-flash not filled: %v %v", updated[0].Context, updated[0].Output)
	}
	if updated[1].Context != 1050000 || updated[1].Output != 128000 {
		t.Errorf("gpt-5.6-luna not filled: %v %v", updated[1].Context, updated[1].Output)
	}
	if updated[2].Context != 1000000 || updated[2].Output != 128000 {
		t.Errorf("existing values not overwritten: %v %v", updated[2].Context, updated[2].Output)
	}
	if updated[3].Context != 500000 || updated[3].Output != "" {
		t.Errorf("grok-4.6 partial fill wrong: %v %v", updated[3].Context, updated[3].Output)
	}
	if updated[4].Context != "64k" || updated[4].Output != "8k" {
		t.Errorf("unknown model should stay untouched: %v %v", updated[4].Context, updated[4].Output)
	}
}

func TestApplyModelPresetsCountsFields(t *testing.T) {
	// Two fields on one card counts twice.
	cards := []ModelCard{{ModelID: "glm-5.3", Context: "", Output: ""}}
	_, filled := ApplyModelPresets(cards)
	if filled != 2 {
		t.Fatalf("filled=%d, want 2", filled)
	}
}

func TestLookupModelLimitsMiMo(t *testing.T) {
	cases := []struct {
		id      string
		wantCtx int
		wantOut int
	}{
		{"mimo-v2.5-pro-2026-08-15", 1048576, 128000}, // snapshot resolves to v2.5-pro family
		{"mimo-vl-7b-sft-2508", 128000, 0},            // open-source VL family, output not published
		{"mimo-7b-rl", 32768, 0},
		{"mimo-audio-7b-instruct", 8192, 0},
		{"mimo-embodied-7b", 128000, 0},
		{"mimo-v2.5-base", 262144, 0}, // exact entry beats mimo-v2.5's 1M prefix
		{"mimo-v2-pro", 1000000, 0},
		{"mimo-v2-omni", 262144, 0},
		{"mimo-v2.5-asr", 8192, 2048},
		{"mimo-v2.5-tts", 8192, 8192},
	}
	for _, c := range cases {
		p, ok := LookupModelLimits(c.id)
		if !ok {
			t.Errorf("%s: not found", c.id)
			continue
		}
		if p.Context != c.wantCtx || p.Output != c.wantOut {
			t.Errorf("%s: got ctx=%d out=%d, want ctx=%d out=%d", c.id, p.Context, p.Output, c.wantCtx, c.wantOut)
		}
	}
	// TTS voice-clone variants have no published limits: matched but nothing to fill.
	p, ok := LookupModelLimits("mimo-v2.5-tts-voiceclone")
	if !ok || p.Context != 0 || p.Output != 0 {
		t.Errorf("mimo-v2.5-tts-voiceclone: %+v ok=%v", p, ok)
	}
}

func TestBuildModelCardsPresetFallback(t *testing.T) {
	build := func(id string, meta map[string]interface{}) ModelCard {
		cards := BuildModelCards([]struct {
			ID   string
			Meta map[string]interface{}
		}{{ID: id, Meta: meta}}, true)
		if len(cards) != 1 {
			t.Fatalf("expected 1 card, got %d", len(cards))
		}
		return cards[0]
	}
	// Provider returned nothing -> preset fills.
	c := build("deepseek-v4-flash", map[string]interface{}{})
	if c.Context != 1000000 || c.Output != 384000 {
		t.Errorf("preset fallback failed: %v %v", c.Context, c.Output)
	}
	if c.APIReturned {
		t.Errorf("APIReturned should stay false when meta empty")
	}
	// Provider returned context -> provider wins, output falls back to preset.
	c = build("deepseek-v4-flash", map[string]interface{}{"context_length": 64000})
	if c.Context != 64000 || c.Output != 384000 {
		t.Errorf("provider priority failed: %v %v", c.Context, c.Output)
	}
	// Provider returned both -> both win.
	c = build("glm-5.3", map[string]interface{}{"max_context_length": 131072, "max_output_tokens": 8192})
	if c.Context != 131072 || c.Output != 8192 {
		t.Errorf("provider values should win: %v %v", c.Context, c.Output)
	}
	// Unknown model with no meta stays empty.
	c = build("custom-local-model", map[string]interface{}{})
	if c.Context != "" || c.Output != "" {
		t.Errorf("unknown model should stay empty: %v %v", c.Context, c.Output)
	}
}

func TestBuildModelCardsAutoFillDisabled(t *testing.T) {
	build := func(id string, meta map[string]interface{}, autoFill bool) ModelCard {
		cards := BuildModelCards([]struct {
			ID   string
			Meta map[string]interface{}
		}{{ID: id, Meta: meta}}, autoFill)
		if len(cards) != 1 {
			t.Fatalf("expected 1 card, got %d", len(cards))
		}
		return cards[0]
	}
	// autoFill off: preset must not fill anything.
	c := build("deepseek-v4-flash", map[string]interface{}{}, false)
	if c.Context != "" || c.Output != "" {
		t.Errorf("autoFill off should keep values empty: %v %v", c.Context, c.Output)
	}
	// Provider-returned values are still respected when autoFill is off.
	c = build("deepseek-v4-flash", map[string]interface{}{"context_length": 64000}, false)
	if c.Context != 64000 || c.Output != "" {
		t.Errorf("provider values should win even with autoFill off: %v %v", c.Context, c.Output)
	}
}
