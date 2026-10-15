package core

import "strings"

// ModelPreset holds preset context/output token limits for a model ID.
// A zero value means that side is not covered by the preset (only the
// covered side gets auto-filled).
type ModelPreset struct {
	Context int
	Output  int
}

// modelPresets maps a lowercase model ID to its token limits, covering the
// model lineups documented by major providers as of 2026-08. Exact IDs win
// first; otherwise the longest key that prefixes the model ID is used, so
// dated snapshots such as "gpt-5.6-luna-2026-..." or "deepseek-v4-flash-0731"
// still resolve to their family's limits.
var modelPresets = map[string]ModelPreset{
	// --- OpenAI ---
	"gpt-5.6-sol":   {1050000, 128000},
	"gpt-5.6-terra": {1050000, 128000},
	"gpt-5.6-luna":  {1050000, 128000},
	"gpt-5.6":       {1050000, 128000},
	"gpt-5.2":       {400000, 128000},
	"gpt-5.1":       {400000, 128000},
	"gpt-5":         {400000, 128000},
	"gpt-5-mini":    {400000, 128000},
	"gpt-4.1":       {1047576, 32768},
	"gpt-4.1-mini":  {1047576, 32768},
	"gpt-4.1-nano":  {1047576, 32768},
	"gpt-4o":        {128000, 16384},
	"gpt-4o-mini":   {128000, 16384},
	"gpt-4-turbo":   {128000, 4096},
	"gpt-4":         {128000, 8192},
	"o3":            {200000, 100000},
	"o4-mini":       {200000, 100000},

	// --- Anthropic Claude ---
	"claude-fable-5":    {1000000, 128000},
	"claude-opus-5":     {1000000, 128000},
	"claude-sonnet-5":   {1000000, 128000},
	"claude-mythos-5":   {1000000, 128000},
	"claude-opus-4-8":   {1000000, 128000},
	"claude-opus-4-7":   {1000000, 128000},
	"claude-opus-4-6":   {1000000, 128000},
	"claude-sonnet-4-6": {1000000, 128000},
	"claude-haiku-4-5":  {200000, 64000},
	"claude-opus-4-5":   {200000, 64000},
	"claude-sonnet-4-5": {200000, 64000},
	"claude-opus-4":     {200000, 32000},
	"claude-sonnet-4":   {200000, 64000},
	"claude-haiku-4":    {200000, 64000},
	"claude-3-7-sonnet": {200000, 64000},
	"claude-3-5-sonnet": {200000, 64000},
	"claude-3-5-haiku":  {200000, 8192},
	"claude-3-opus":     {200000, 4096},
	"claude-3-haiku":    {200000, 4096},
	"claude-3-sonnet":   {200000, 4096},

	// --- Google Gemini ---
	"gemini-3.7-flash":       {1048576, 65536},
	"gemini-3.6-flash":       {1048576, 65536},
	"gemini-3.5-flash":       {1048576, 65536},
	"gemini-3.5-flash-lite":  {1048576, 65536},
	"gemini-3.1-pro-preview": {1048576, 65536},
	"gemini-3.1-flash-lite":  {1048576, 65536},
	"gemini-3-flash-preview": {1048576, 65536},
	"gemini-3-pro-preview":   {1048576, 65536},
	"gemini-2.5-pro":         {1048576, 65536},
	"gemini-2.5-flash":       {1048576, 65536},
	"gemini-2.5-flash-lite":  {1048576, 65536},
	"gemini-2.0-flash":       {1048576, 8192},
	"gemini-2.0-flash-lite":  {1048576, 8192},
	"gemini-1.5-pro":         {1048576, 8192},
	"gemini-1.5-flash":       {1048576, 8192},

	// --- DeepSeek ---
	"deepseek-v4-flash":            {1000000, 384000},
	"deepseek-v4-pro":              {1000000, 384000},
	"deepseek-v4-flash-vision-exp": {1000000, 384000},
	"deepseek-chat":                {128000, 8000},
	"deepseek-reasoner":            {128000, 64000},
	"deepseek-v3":                  {128000, 8000},
	"deepseek-r1":                  {64000, 8000},
	"deepseek-r1-0528":             {64000, 32000},

	// --- Zhipu GLM ---
	"glm-5.3":              {1000000, 128000},
	"glm-5.2":              {1000000, 128000},
	"glm-5.1":              {200000, 128000},
	"glm-5":                {200000, 128000},
	"glm-5-turbo":          {200000, 128000},
	"glm-4.7":              {200000, 128000},
	"glm-4.7-flash":        {200000, 128000},
	"glm-4.7-flash-x":      {200000, 128000},
	"glm-4.6":              {200000, 128000},
	"glm-4.6-air":          {200000, 32000},
	"glm-4.5":              {128000, 96000},
	"glm-4.5-air":          {128000, 96000},
	"glm-4.5-air-x":        {128000, 96000},
	"glm-4.5-flash":        {128000, 96000},
	"glm-4-flash-250414":   {128000, 16000},
	"glm-4-flash-x-250414": {128000, 16000},
	"glm-4-long":           {1000000, 4000},
	"glm-5v-turbo":         {200000, 128000},
	"glm-4.6v":             {128000, 32000},
	"glm-4":                {128000, 4000},
	"glm-4-plus":           {128000, 32000},
	"glm-4-air":            {128000, 32000},

	// --- Alibaba Qwen ---
	"qwen3.8-max":          {1000000, 131000},
	"qwen3.7-max":          {1000000, 131000},
	"qwen3.7-plus":         {1000000, 131000},
	"qwen3.7-flash":        {1000000, 131000},
	"qwen3.6-max-preview":  {256000, 65000},
	"qwen3.6-plus":         {1000000, 65000},
	"qwen3.6-flash":        {1000000, 65000},
	"qwen3.5-plus":         {1000000, 65000},
	"qwen3.5-flash":        {1000000, 65000},
	"qwen3-max":            {256000, 65000},
	"qwen3-coder-plus":     {1000000, 65000},
	"qwen3-coder-flash":    {1000000, 65000},
	"qwen3-coder-next":     {256000, 65000},
	"qwen-plus":            {1000000, 32768},
	"qwen-max":             {32000, 8000},
	"qwen-turbo":           {128000, 16000},
	"qwen-flash":           {1000000, 32000},
	"qwen-long":            {10000000, 0},
	"qwen2.5-max":          {32000, 8000},
	"qwen2.5-plus":         {128000, 8000},
	"qwen2.5-flash":        {128000, 8000},
	"qwen2.5-coder":        {128000, 8192},
	"qwen2.5-72b-instruct": {128000, 8000},
	"qwen3-coder":          {256000, 8000},
	"qwen3-plus":           {128000, 8000},
	"qwen3-flash":          {128000, 8000},

	// --- Moonshot Kimi ---
	"kimi-k3":                  {1048576, 128000},
	"kimi-k2.7-code":           {262144, 32000},
	"kimi-k2.7-code-highspeed": {262144, 32000},
	"kimi-k2.6":                {262144, 32000},
	"kimi-k2.5":                {262144, 32000},
	"moonshot-v1-8k":           {8192, 0},
	"moonshot-v1-32k":          {32768, 0},
	"moonshot-v1-128k":         {131072, 0},
	"kimi-k2":                  {128000, 16000},
	"kimi-k2-thinking":         {128000, 32000},
	"kimi-latest":              {128000, 16000},

	// --- MiniMax ---
	"minimax-m3":             {1000000, 0},
	"minimax-m2.7":           {200000, 128000},
	"minimax-m2.5":           {200000, 32000},
	"minimax-m2.5-highspeed": {200000, 32000},
	"minimax-m2.1":           {200000, 128000},
	"minimax-m2":             {200000, 128000},
	"minimax-m1":             {1000000, 32000},

	// --- xAI Grok ---
	"grok-4.6":                     {500000, 0},
	"grok-4.5":                     {500000, 0},
	"grok-4.3":                     {1000000, 0},
	"grok-4.20-0309-reasoning":     {1000000, 0},
	"grok-4.20-0309-non-reasoning": {1000000, 0},
	"grok-4.20-multi-agent-0309":   {1000000, 0},
	"grok-4":                       {256000, 128000},
	"grok-3":                       {131000, 32000},

	// --- Mistral ---
	"mistral-large-3":          {256000, 0},
	"mistral-large-latest":     {256000, 0},
	"mistral-medium-3-5-26-04": {256000, 0},
	"mistral-medium-2604":      {256000, 0},
	"mistral-small-4-0-26-03":  {256000, 0},
	"ministral-3-8b":           {256000, 0},
	"ministral-3-3b":           {256000, 0},
	"ministral-3-14b":          {256000, 0},
	"codestral":                {128000, 0},
	"mistral-large-2":          {128000, 128000},

	// --- Meta Llama ---
	"llama-4-maverick":        {1000000, 0},
	"llama-4-scout":           {1000000, 0},
	"llama-3.3-70b-instruct":  {128000, 8000},
	"llama-3.1-8b-instruct":   {128000, 8000},
	"llama-3.1-70b-instruct":  {128000, 8000},
	"llama-3.1-405b-instruct": {128000, 8000},
	"llama-3.2-1b":            {128000, 8000},
	"llama-3.2-3b":            {128000, 8000},
	"llama-3.2-11b":           {128000, 8000},
	"llama-3.2-90b":           {128000, 8000},

	// --- ByteDance Doubao (Volcengine Ark) ---
	"doubao-seed-evolving":                {1024000, 256000},
	"doubao-seed-2-1-pro-260628":          {256000, 256000},
	"doubao-seed-2-1-turbo-260628":        {256000, 256000},
	"doubao-seed-2-0-pro-260215":          {256000, 128000},
	"doubao-seed-2-0-lite-260428":         {256000, 128000},
	"doubao-seed-2-0-mini-260428":         {256000, 128000},
	"doubao-seed-2-0-code-preview-260215": {256000, 128000},
	"doubao-seed-character-260628":        {128000, 32000},
	"doubao-seed-1-8-251228":              {256000, 32000},
	"doubao-seed-code-preview-251028":     {256000, 32000},
	"doubao-seed-1-6-flash-250828":        {256000, 32000},
	"doubao-seed-1-6-vision-250815":       {256000, 32000},
	"doubao-seed-1-6-251015":              {256000, 32000},
	"doubao-1-5-pro-32k-250115":           {32000, 8000},
	"doubao-1-5-lite-32k-250115":          {32000, 8000},
	"doubao-1-5-vision-pro-32k-250115":    {32000, 8000},

	// --- Tencent Hunyuan ---
	"hunyuan-a13b":                {224000, 32000},
	"hunyuan-t1":                  {224000, 16000},
	"hunyuan-turbos-latest":       {224000, 16000},
	"hunyuan-large":               {256000, 16000},
	"hunyuan-code":                {128000, 8000},
	"hunyuan-vision-1.5-instruct": {24000, 16000},
	"hunyuan-t1-vision-20250916":  {28000, 20000},

	// --- Step (阶跃星辰) ---
	"step-3.7-flash": {256000, 0},
	"step-3.5-flash": {256000, 0},
	"step-2":         {200000, 0},

	// --- Baichuan ---
	"baichuan4":              {128000, 0},
	"baichuan4-turbo-250715": {128000, 0},

	// --- 01.AI Yi ---
	"yi-lightning":   {16000, 0},
	"yi-vision-v2":   {16000, 0},
	"yi-medium-200k": {200000, 0},

	// --- iFlytek Spark ---
	"4.0ultra":    {32000, 32000},
	"max-32k":     {32000, 32000},
	"pro-128k":    {128000, 128000},
	"generalv3.5": {8000, 8000},
	"lite":        {8000, 4000},

	// --- Cohere ---
	"command-a-plus-05-2026":      {128000, 64000},
	"command-a-03-2025":           {256000, 8000},
	"command-a-reasoning-08-2025": {256000, 32000},
	"command-r-plus-08-2024":      {128000, 4000},
	"command-r-08-2024":           {128000, 4000},

	// --- Xiaomi MiMo ---
	"mimo-v2.5-pro":             {1048576, 128000},
	"mimo-v2.5":                 {1048576, 128000},
	"mimo-v2.5-dflash":          {1048576, 0},
	"mimo-v2.5-asr":             {8192, 2048},
	"mimo-v2.5-tts":             {8192, 8192},
	"mimo-v2.5-tts-voiceclone":  {0, 0},
	"mimo-v2.5-tts-voicedesign": {0, 0},
	"mimo-v2.5-base":            {262144, 0},
	"mimo-v2.5-pro-base":        {262144, 0},
	"mimo-v2-pro":               {1000000, 0},
	"mimo-v2-omni":              {262144, 0},
	"mimo-v2-flash":             {262144, 65536},
	"mimo-v2-flash-base":        {262144, 0},
	"mimo-7b":                   {32768, 0},
	"mimo-vl-7b":                {128000, 0},
	"mimo-audio-7b":             {8192, 0},
	"mimo-embodied-7b":          {128000, 0},
}

// LookupModelLimits returns the preset limits for modelID. It matches the
// lowercase ID exactly first, then falls back to the longest key that is a
// prefix of the ID (covers dated snapshot IDs).
func LookupModelLimits(modelID string) (ModelPreset, bool) {
	low := strings.ToLower(strings.TrimSpace(modelID))
	if low == "" {
		return ModelPreset{}, false
	}
	if p, ok := modelPresets[low]; ok {
		return p, true
	}
	best := ModelPreset{}
	bestLen := 0
	for key, p := range modelPresets {
		if len(key) > bestLen && strings.HasPrefix(low, key) {
			best, bestLen = p, len(key)
		}
	}
	if bestLen == 0 {
		return ModelPreset{}, false
	}
	return best, true
}

// emptyToken reports whether a card context/output value is unset.
func emptyToken(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case int:
		return t <= 0
	case int64:
		return t <= 0
	case float64:
		return t <= 0
	case float32:
		return t <= 0
	}
	return false
}

// ApplyModelPresets applies the inferred context/output limits to the cards, overwriting
// any existing context/output values the inference covers. The caller is expected to
// confirm with the user before overwriting filled values.
// It returns the updated cards and the number of fields set.
//
// 取值口径与拉取模型时的自动补全完全一致：走 LookupModelCapability（APM 覆盖层 >
// ZCode 内建规则 > presets 旧表兜底），而不是只查手维护的 modelPresets 表——否则同一个
// 模型在「拉取」与「自动匹配」两条路径上会得到不同结果（例如只在覆盖层里的
// deepseek-v4.1-flash：拉取能补全，自动匹配却一格不填）。
//
// 保留函数名与签名：它已是 Wails 绑定名（前端 api.ApplyModelPresets），改名要同时动
// 前端与绑定层，收益不抵。
//
// 本函数没有站点上下文，等价于 ApplyModelPresetsForSite(cards, "", "")；知道 provider
// 的 baseUrl / api.type 时改用后者，让离线目录与站点级规则一起参与。
//
// filled 仍只统计 context/output 两类字段（档位补全属于卡片自适应，另行提示）。
func ApplyModelPresets(cards []ModelCard) ([]ModelCard, int) {
	return ApplyModelPresetsForSite(cards, "", "")
}

// ApplyModelPresetsForSite 是带站点上下文的「自动匹配」：除覆盖层 / 内建正则 / presets
// 旧表外，离线精确目录与站点级规则（例如 opencode 网关的专属档位）也参与取值。
//
// 语义与 ApplyModelPresets 完全一致（覆盖式写入 ctx/out，filled 计字段数），只是把
// baseURL / apiType 透传给 LookupModelCapabilityForSite。baseURL 为空时与前者逐字段等价。
// apiType 取不到时传空串：限定 apiType 的站点规则会被跳过（fail-closed），而不是误命中。
//
// 中立能力位（capabilities）用同一次分层求值的结果一并填充，但采取**只补空缺**的语义
// （卡片已有选择一律保留），与前端「按能力表补全」按钮同一口径——ctx/out 是估算值可以覆盖，
// 能力位则可能已经被用户逐位确认过，覆盖等于抹掉用户的选择。能力位不计入 filled
// （filled 仍是 ctx/out 的字段数，前端有独立的能力入口与计数）。
func ApplyModelPresetsForSite(cards []ModelCard, baseURL, apiType string) ([]ModelCard, int) {
	filled := 0
	for i := range cards {
		cap := LookupModelCapabilityForSite(cards[i].ModelID, baseURL, apiType)
		if cap.Context > 0 {
			cards[i].Context = cap.Context
			filled++
		}
		if cap.Output > 0 {
			cards[i].Output = cap.Output
			filled++
		}
		if cap.Features.HasAny() {
			var cur ModelFeatures
			if cards[i].Capabilities != nil {
				cur = *cards[i].Capabilities
			}
			if merged := mergeFeatures(cur, cap.Features); merged.HasAny() {
				cards[i].Capabilities = &merged
			}
		}
	}
	return cards, filled
}
