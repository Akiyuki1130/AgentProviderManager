# Changelog

## 2.1.1-pre — 2026-10-14

### 新增

- **版本号单一真源。** 仓库根目录的 `VERSION` 文件现在驱动版本出现的每一处，由 `scripts/sync-version.mjs` 分发（带 `--check` 模式）。Go 侧读取生成的 `internal/core/version_gen.go`（`core.AppVersion`，由 `main.go` 与 `GetAppInfo` 使用），侧栏不再硬编码：`AppSidebar.vue` 启动时从 `GetAppInfo` 读取运行版本。`go test ./...` 新增 `TestVersionSourcesInSync`，一旦与 `VERSION` 漂移即失败并打印修法，因此发版只需改 `VERSION` 再跑一次脚本（覆盖 `wails.json`、`frontend/package.json` 及其字节一致的 `package.json.md5`（Wails 用它判断是否重装依赖）、`frontend/package-lock.json` 与 `README.md`）。
- **两段式连接探测（`测试连接`）。** `core.TestConnection` 先复用加固过的取模型链路（SSRF 门、重定向姿态、错误码）访问 `/v1/models`，再用首个发现的模型向该协议的真实端点（`/chat/completions` / `/messages`，后者带 `x-api-key` + `anthropic-version` / `/responses`）发送 1-token 对话请求，因此鉴权是在真正承载流量的端点上得到验证的。响应按段报告 ok/延迟/数量，沿用既有的 `ModelFetchError` 错误码分类（DNS `CONN`、`TIMEOUT`、`HTTP_401`/`HTTP_404`、`BAD_URL`），所有消息都经 `RedactSensitive` 处理。以 `TestConnectionRaw`（表单值）与 `TestProviderConnection`（对已存 provider 走与 `RefreshProviderModels` 相同的后端凭据解析，DSH 的 `apiKeyEnv` 密钥可透明解析）暴露；已接入提供商编辑区与导入页，内联显示 ok/warn/err 结果行。
- **钥匙串 ↔ 提供商联动。** API Key 字段新增 `从钥匙串`（对受 DPAPI 保护的条目做可搜索的拾取弹窗并填入字段）与 `存入钥匙串`（把当前密钥存为具名条目）。所选关联记录在应用侧 `settings.json`（`keychain_links`，`agent|providerID` → 条目 id——ZCode 严格的 v2 schema 禁止在 `provider_config.json` 内做间接引用，因此 harness 配置仍保留解析后的真实值），在字段下方以同步 chip 呈现（覆盖 / 更新钥匙串 / 解除），钥匙串行上显示 `N 提供商` 徽标。绑定：`GetKeychainLinks`、`SetKeychainLink`、`RemoveKeychainLink`。
- **可移植备份导出 / 导入（`apm-backup v1`）。** 还原点页现在把当前 Agent 的提供商（全部或指定）导出为 JSON 信封，并经既有 `ImportProvider` 路径导回，因此每个导入的提供商都会自动获得自己的备份 + 还原点。是否含密钥与加密方式是两个正交选项：不含密钥（默认，可安全分享——`StripBackupKeys` 清除 `APIKey`、`options_json` 内的 `apiKey` 以及原始行）或含密钥；载荷可封为明文（plain——含密钥导出需要一次显式的“仍然导出”确认弹窗）、默认加密（sealed：以程序内置种子的 SHA-256 为密钥的 AES-256-GCM，任意设备上的本程序均可解密，能防止被直接当作明文读取，但不防有人提取该种子）或密码加密（标准库 `crypto/pbkdf2` SHA-256，60 万次迭代 + AES-256-GCM；元数据在解锁前保持锁定——`InspectBackup` 返回 `needs_password`，错误密码会在 GCM tag 上失败）。用不含密钥的备份覆盖已有提供商时会保留其本地 API Key（与 `MergeConfig` 的保护一致），自定义 headers 等非敏感 `options_json` 能经导入往返。文件以 0600 写入；密码绝不落盘。
- 管理页的提供商侧栏过滤与排序：一个匹配名称/id 的搜索框加一个排序下拉（默认顺序 / 名称 / 模型数），表头计数切换为 `filtered / total`。

### 变更（UI）

- **UI 中不再有任何 emoji（i18n 字典里也没有）。** 所有图形化字形（按钮、弹窗标题、徽标、`✔✕⚠` 状态字符、混进翻译字符串里的 emoji 前缀）都被替换为 `@vicons/ionicons5` 图标，经 naive-ui `<n-icon>` 渲染在按钮的 `#icon` 插槽里，因此字形跟随 accent 色、共享基线并随 size 属性缩放。对 `frontend/src` 做全量 emoji 码点扫描现在返回 0。
- **八处原生 OS 弹窗全部移除。** `prompt()` / `confirm()` 被主题化弹窗宿主（`components/ui/UiDialogHost.vue` + `stores/uiDialog.ts`，暴露 `uiConfirm` / `uiPrompt`）取代，在 `App.vue` 里挂载一次。删除类确认把正按钮样式化为 `error`（`danger: true`）；文本输入框内联校验并回车提交。UI 无法再触发 OS 绘制的 prompt、confirm 或 alert。
- **可复用组件 + composable 层。** 新增 `ui/PageHeader.vue`（标题 / 副标题 / 图标 / actions，被五个卡片式页面采用——导入 / 钥匙串 / 迁移 / 选项 / 还原点）、`ui/StickySaveBar.vue`（管理页的脏态保存条），以及 composable `useToast`（hover 暂停倒计时、语义图标、关闭按钮、最多 5 条、`aria-live="polite"`）、`useStagger`（按稳定 id 缓存错峰延迟，列表换序不会重播动画）、`useHighlight`（大小写不敏感的 `<mark>` 搜索高亮，渲染为转义分段——不用 `v-html`）。
- **设计 token 层。** 动效 token（`--dur-standard` / `-lg` / `--dur-settle` / `--dur-toast` / `--dur-page-leave` / `--dur-reveal` / `--ease-standard` / `--delay-step`）替换了全站硬编码时长；语义色集（`--ok` / `--warn` / `--err` 及 `-soft` 色调变体，含明暗两套色板）替换了五处临时硬编码的绿/红/橙。圆角改为比例缩放（控件高度 ×≈0.28：tiny 6 / small 8 / medium 10 / large 12 / 容器 12），不再用各页松散取值。新增 FLIP 列表与网格过渡（`.list-*`、`.grid-*`）以及共享的 `fade-in` / `.enter-stagger` 处理列表重排与页面入场。
- **`global.css` 里的规范控件样式。** `.field`、`.switch`、`.check`、`.badge`（三种语义变体）、`.pill` 现在有单一全局定义，页面复用而非重复声明，修掉了深色主题缺陷（不可见的开关轨道、写死的 `#888` 勾选边框），改用主题派生的 `color-mix` 颜色，补上 `:active` 状态，并让每个自绘控件都带 `role` / `aria-checked` / `tabindex` 及 Enter/Space 处理。
- **基础打磨。** 全局 `:focus-visible` 聚焦环（对自带聚焦态的 naive-ui 控件抑制）；`tabular-nums` / `mono` 工具类，以及为路径、ID、日志提供豁免全局禁选的 `.selectable`；隐藏滚动条；并用 `index.html` 中一段同步的 `<head>` 脚本在 CSS 包加载前涂好正确底色（消除冷启动白闪）。
- **深色主题与布局修复。** `theme.ts` 新增 `accentTextOnAccent`（按 WCAG 亮度在 accent 上取黑/白）、深色下按压态改为变亮而非变暗、以及按尺寸区分的按钮圆角；顶栏重新可整体拖拽（拖拽作用于整个头部，`no-drag` 收窄到真正的控件，因此路径胶囊也能随之拖动）；目标路径胶囊现在渐进折叠中间段（`盘符\…\file`）以便长路径可读，完整路径放在 tooltip 里；窗口控制与工具栏按钮在容器宽度低于 1080px 时收起文字标签（纯图标，经 `ResizeObserver` + 容器查询）；选项页用 accent 色板拾取器（八个预设 + 自定义）替换了原始颜色输入。
- **行为修正（仅前端）。** 选项页的开关现在返回后端结果并在失败时用错误 toast 回滚，而不再先弹成功 toast 再吞掉错误；钥匙串页与导入页显示区分开的 loading 与错误态，不再把失败伪装成“暂无条目”；批量上下文/输出输入框改文案为“输入后回车应用”并在回车时校验；钥匙串的显示标签与实际的全局显示状态一致。
- **其余页面的 i18n 补齐。** `KeychainPage` 与 `MigratePage` 完全接入 `t()`，导入页的硬编码字符串移入字典，`main.ts` 里的全局错误 toast 也已本地化；三套字典（zh / en / ja）保持同步，字符串本身的 emoji 前缀已移除（图标留在模板层）。
- **按 Agent 门控的 ZCode 区块。** ZCode 配置格式卡与旧版导入卡是 ZCode 独有的概念，在 DeepSeek Harness 下不再渲染（也不探测）；后续区块相应前移其错峰入场序号，避免出现动画空档。

### 修复

- `go test` 不再污染真实还原点目录：`internal/core` 里的 `TestZCodeToDeepSeek...`、`TestDeepSeekToZCode...` 与三个 `ApplyLegacyImport` 测试此前只沙箱化了 `DSH_HOME`，把迁移/导入快照写进了 `%LOCALAPPDATA%\AgentProviderManager\restorepoints`。现在它们把 `LOCALAPPDATA`（以及 `USERPROFILE`，并为反向迁移的目标存储列表清空 `ZCODE_PERSONAL_PROVIDER_CONFIG_FILE`）固定到 `t.TempDir()`，与 `setupMigrateTest` 一致。

## 2.0.8 — 2026-10-13

### Added

- **DeepSeek Harness 0.2 configuration backend.** DSH 0.2 (today's desktop app and the CLI `web` profile) keeps user configuration in per-profile patch layers, so the agent now has real config backends instead of hardcoded branches: `~/.dsh/profiles/desktop/cordis.patch.yml` (desktop), `~/.dsh/profiles/web/cordis.patch.yml` (`dsh web` / `dsh --profile web`), any other `profiles/<name>/cordis.patch.yml`, and the home-level `~/.dsh/cordis.patch.yml`. Providers are read from and written to the `id: llm-pi-ai` entry's `config.providers` (plugin `@deepseek-ai/dsh-llm-pi-ai`); entries are created the same way DSH's own config editor creates them, and the surrounding rows, comments and `!!js` expressions are preserved because the patch is edited as a YAML node tree.
- `internal/dshprovider`: format detection (desktop / web / custom profile / legacy settings), profile-patch read/write with `yaml.v3` node trees, and the `version: 1` + `refs`/`records` credentials document. A 0.1 flat credentials file is upgraded to the versioned layout on write and existing `records`/comments survive.
- Config location detection for DeepSeek Harness now lists the desktop patch, the web patch, every other discovered profile patch and the home-level patch alongside the legacy `settings.*` files, with their real provider counts. A fresh install prefers the 0.2 desktop patch on startup, and an already-configured 0.1 `settings.yaml` target is switched to the 0.2 profile patch when one exists.

### Changed

- The backend registry is no longer ZCode-specific: `RegisterBackendForFormat`/`RegisterFormatDetector` take a `BackendFormat`, so ZCode (v2/legacy) and DeepSeek Harness (profile/settings) both route through `GetBackend`.
- `ListProviders`, `GetProvider`, `SaveProvider`, `DeleteProvider`, `DeleteModel`, `RefreshProviderModels`, `ImportProvider`, target validation and the config IO helpers now go through `GetBackend` for every agent that registers backends; the shelved OpenCode keeps its generic JSON path but has no UI entry point. Responses carry a `format` field so the caller can tell a 0.2 profile patch from a 0.1 `settings.yaml`.
- DeepSeek provider entries are encoded in one place (`deepSeekBuildEntry`): a route always declares its pi-ai protocol (`openai-completions` / `openai-responses` / `anthropic-messages`), the API key becomes an `apiKeyEnv` reference and the secret is written to `.credentials.yaml`; renaming a provider follows `agent-default-model` in both the legacy settings document and the profile patch.
- Cross-agent migration is now backend-driven on both sides: source and target are resolved with `GetBackend`, providers are read as `ProviderEdit` and written through the target's `Store.Upsert`, and previews list each side from the file the user actually selected (falling back to the new-format-first default paths). ZCode (new `provider_config.json` / legacy `config.json`) and DeepSeek Harness (0.2 profile patch / 0.1 `settings.yaml`) can be migrated into each other in both directions and in both merge and overwrite mode; the old `deepSeekMigratePathGuard` refusal (and the `AgentOpenCode` default target) are gone. Migrating into a profile patch that a home-level patch shadows still fails with the explicit override error, and a patch's comments / `!!js` rows survive because the write goes through the YAML node-tree editor.

### Changed (UI)

- OpenCode is shelved and no longer offered anywhere in the app: the agent switcher, the migration page and the OpenCode-only editor rows are gone, and a stored `agent: opencode` setting falls back to ZCode on load. The `AgentOpenCode` constant, the conversion/import code and the historical restore-point badge/label stay in place.

### Fixed

- Switching agents in the top bar used to point the target at the legacy file (`config.json` for ZCode, `settings.yaml` for DeepSeek Harness) even when the new format existed; it now resolves the target exactly like startup does (new-format-first), so switching to DeepSeek Harness lands on the 0.2 profile patch instead of the 0.1 settings file.
- `MergeConfig` / `PreviewConfigMerge` (merging providers out of a chosen configuration file) no longer use the legacy-only provider map: they detect the source file's agent/format, read it through the matching backend and write through the target's backend, so a ZCode new-format target stays strictly decodable, a DeepSeek Harness 0.2 target is no longer rejected, and API keys are staged into the right place (`.credentials.yaml` for DSH).
- Historical restore points created for OpenCode still show `OpenCode` as their agent label now that `AllAgents()` no longer contains it.
- **A fresh ZCode install could write a legacy provider map into `provider_config.json`.** When the new-format file did not exist yet (or existed but was empty), content-based format detection returned `unknown` and the legacy backend took over, so the first `SaveProvider` / `ImportProvider` / migration produced `{"provider": {...}}` inside `provider_config.json`. ZCode's `.strict()` schema rejects that document and the agent behaves as if the whole personal configuration were empty. `provider_config.json` (the default location, the `ZCODE_PERSONAL_PROVIDER_CONFIG_FILE` override, or any file with that name) is now routed to the v2 backend by path as well as by content, and an empty new-format file is read as an empty configuration instead of failing with `EOF`.
- Writing a profile patch no longer loses `!!js` expressions, comments, or key order inside the `llm-pi-ai` entry's `config`: `SetProviders`/`SetAgentDefaultModel` now update only the changed nodes of the YAML tree instead of decoding the whole `config` into a map and re-encoding it (which silently turned `!!js process.env.X` into the literal string `process.env.X`). Provider entries whose values actually changed are still re-encoded.
- The home-level `$DSH_HOME/cordis.patch.yml` is recognised as a patch even when it does not exist yet or is empty / comment-only. Previously such a file was classified as unknown, fell back to the 0.1 settings backend, and a save rewrote it with the `llm-pi-ai:` settings layout, which DSH 0.2 cannot read.
- Model discovery no longer sends an empty `Authorization: Bearer ` header when a redirect returns to the original host with no API key, and the redirect-auth code cannot panic on an unparsable URL. Cross-host redirects still drop the API key.
- Reasoning variant lists read from a model map are now sorted in the canonical UI order, so the same configuration no longer produces a different variant order (and a rewritten file) on every read.
- Private/reserved address filtering now strips an IPv6 zone id (`fe80::1%eth0`, `::1%eth0`, `fd00::1%eth0`) before parsing, closing a bypass in the SSRF pre-check.
- The updater's helper argument replacement is deterministic (fixed placeholder order, single pass) instead of iterating a map.
- `ImportProvider` now reports the model count of the written provider, so a merge that keeps existing models reports the final total instead of only the submitted cards.
- Cross-agent migration keeps provider-level extras that both sides can express: a ZCode new-format provider's `api.headers` now lands in the DSH route's `headers`, and a DSH route's `headers` come back as `api.headers` (the DSH store exposes the pi-ai profile's passthrough fields through `options_json`, the v2 store lifts `options.headers` into the rule, and unknown keys are still never copied into a DSH patch).
- A write that is refused because the home-level patch shadows `llm-pi-ai` no longer leaves an empty restore point and a sibling `.bak_` file behind: the override is detected before the snapshot is taken.
- The `ImportProvider` guard for "empty model list with merge_models=false" now reports its own message ("为避免误删已有模型，已取消导入") instead of being unreachable behind the generic `BuildProviderCfg` validation error.
- DeepSeek credentials were written in the pre-0.2 flat layout, which DSH 0.2 rejects at startup (`uses the pre-release flat layout`); writes now always produce `version: 1` with a `refs` section while preserving `records`.
- An `llm-pi-ai` configuration in the home-level patch shadows the profile layer; the app now reports the effective providers from the home patch and refuses writes/deletes that would silently have no effect.

## 2.0.7 — 2026-10-03

### Fixed

- **Importing a provider into a ZCode new-format file could make ZCode treat the whole personal configuration as empty.** `ImportProvider` resolved its target outside the config backends: it read the file as a plain provider map, merged the incoming provider into a top-level `provider` key with `MergeProviderIntoConfig`, and wrote the result back through the generic JSON writer. For `provider_config.json` that produced a document with an extra top-level `provider` key, which ZCode's `.strict()` schema rejects (`unrecognized_keys: ["provider"]`); ZCode kept the file on disk but fell back to an in-memory empty configuration, so every provider disappeared from the agent. The ZCode path now goes through `GetBackend` → `Store.Upsert` → `Backend.Write`, like every other ZCode mutation, so new-format files are only ever written from `zcodeprovider.Encode`.
- Writes to a new-format ZCode file are now guarded in one place: `WriteConfigBytes` refuses any payload that looks like a new-format document (top-level `schemaVersion` and `config`) but does not pass the strict ZCode validation, and explains why. This closes the same corruption in the flows that are still legacy-only — OpenCode import, configuration merge and cross-agent migration now fail with a clear error and leave the file untouched instead of writing something ZCode rejects.

### Added

- `core.MergeModelCards`: `merge_models=true` keeps its previous semantics (existing models are preserved, same-ID models are replaced, new IDs are appended) on the new-format backend, whose `Upsert` replaces a provider's model set wholesale.
- Regression tests: importing into a new-format file must leave it strictly valid, merging must keep existing models, a legacy `config.json` target must keep its previous shape, the legacy-only flows must refuse a new-format target without touching it, and the write guard must reject a new-format document with an unknown top-level key while leaving legacy-shaped JSON for other agents alone.

## 2.0.6 — 2026-10-03

### Added

- Restore points: before every change to an agent configuration file the previous file is now kept as a restore point under `%LOCALAPPDATA%\AgentProviderManager\restorepoints`, together with its metadata — agent, absolute target path, detected format (`v2` / `legacy` / `opencode` / `deepseek` / `unknown`), whether the file existed at all, byte size, SHA-256, fingerprint, app version, the operation that caused it, the affected provider/model, and a note. The content is a byte-for-byte copy, so comments, indentation and key order of the previous file survive. Every write path goes through a single `PrepareChange` entry point, and the sibling `.bak_<timestamp>` backup is still written.
- New **Restore Points** page: lists the history (time, agent, format badge, target path, operation, affected provider/model, size, "did not exist yet"), restores any single point, deletes one point, opens the storage directory, and prunes old points. Restoring first snapshots the current state, so a restore can itself be undone. Restoring a point that recorded "the file did not exist" renames the current file to `<file>.removed_<timestamp>` instead of deleting it.
- ZCode compatibility check and one-click repair: problems that ZCode 3.14 rejects but this tool can still read are listed with their path and reason, and can be repaired in one step (the repair creates a restore point first).

### Changed

- Aligned the ZCode new-format (`provider_config.json`, `schemaVersion` 1) rules with ZCode 3.14: a personal provider rule no longer accepts `builtinModelIds` (the repair merges the value into `personalModelIds`), `config.group` must be `standard-personal` or absent/null (the family values only exist in ZCode's built-in file), `config.logo` must be `{"type":"builtin","key":"…"}` or null, `api` / `access` / `api.baseUrl` / `access.apiKey` / `access.apiKeyManagementUrl` / `providerName` / `templateId` may be absent or null, `providerId` must be unique, and the same `(providerId, modelId)` must not appear in both `providerModelRules` and `manualProviderModelRules`. Writing always enforces these rules, so a save can no longer produce a file that makes ZCode treat the whole configuration as empty.
- New-format files are written in ZCode's own key order, so ZCode no longer rewrites the file on its next read.
- Reading a new-format file is now lenient about the compatibility problems above: they are reported instead of making the configuration impossible to open.
- Legacy → new mapping now follows ZCode's own migration more closely: `apiFormat` / `defaultKind` are preferred when deciding `api.type`, `endpoints.baseURL` and the top-level `api` string are used as base URL fallbacks, `headers` → `api.headers`, `apiKeyUrl` → `access.apiKeyManagementUrl`, provider `enabled` is carried over, model-level `contextWindow` / `maxOutputTokens` take precedence over `limit.context` / `limit.output`, and models marked `deleted` are skipped. The "will be lost" list stays accurate (for example `endpoints.paths` is reported because the new format has no place for path suffixes).

### Fixed

- A `schemaVersion` 1 file containing keys or values that ZCode 3.14 rejects no longer makes the provider list unreadable; the reason is shown and the file can be repaired in place.
- Rolling back a configuration no longer requires guessing: every change can be reverted to any earlier state instead of only the most recent sibling backup.

## 2.0.5 — 2026-09-18

### Added

- ZCode new-format support: reads and writes `~/.zcode/v2/provider_config.json` (`schemaVersion` 1), with `ZCODE_PERSONAL_PROVIDER_CONFIG_FILE` overriding the location, and detects the target file's format (`v2` / `legacy` / `unknown`) by inspecting its contents.
- One-time import of legacy providers into the new format, with a per-provider list of fields the new format cannot carry, an explicit confirmation step, and an automatic backup of the new config before writing.
- Automatic switch to the new config path on startup when `provider_config.json` already exists. The new file is also reported as a selectable ZCode config location with its real provider count.
- Legacy model limits and reasoning options now carry over to the new schema: `limit.context` → `properties.contextWindow`, `limit.output` → `optionSpecs.maxOutputTokens.max`, and `reasoning.variants` → `optionSpecs.reasoningLevel.values` (the legacy `off` level is renamed to `disabled`). Legacy `modalities` map to `properties.inputFormat`.
- Fields the new schema has no representation for (`source`, `zcode.modified`, `zcode.priority`, `apiKeyRequired`, unknown `options` keys) are not written when saving to the new format; the import preview lists them per provider.

### Fixed

- A provider with no models no longer breaks the editor: the backend returned a `null` model list, which threw on `cards.map` right after the success toast and left the editor's saved state stale, so the next save could write pre-save values back. The provider list and the per-model reasoning variants had the same null gap and are fixed with it.
- Editing a provider in the new format preserves ZCode-only fields this tool does not edit (`templateId`, `enabled`, `logo`, `visibility`, `modelOrder`, `builtinModelIds`, `api.headers`, `apiKeyManagementUrl`, `optionSpecs.*.map`), instead of dropping them on save.
- Renaming a provider now renames its model rules and `providerOrder` entry together, and no longer overwrites an existing provider ID.
- Deleting a model now removes it from `personalModelIds` as well as from its model rule.
- The cross-agent migration page lists ZCode providers again for users on the new format.
- Legacy-provider imports and their backup now target the currently selected new-format file rather than always the canonical path.
- Legacy import preview and apply now skip ZCode-managed providers (`builtin:` / `account:` prefixes) instead of importing them as personal providers.

## 2.0.4 — 2026-08-31

### Added

- New "Allow local & private addresses" option (off by default). When enabled, model discovery can reach localhost, loopback, LAN, and other private/reserved hosts such as local model servers (e.g. `http://127.0.0.1:8787/v1`). Requires the existing plain-HTTP option for `http://` URLs. The updater's GitHub-only host validation is unaffected.

## 2.0.3 — 2026-08-22

### Fixed

- Preserved reasoning enabled state when converting between ZCode, OpenCode, and DeepSeek Harness.
- Preserved the selected default reasoning effort during cross-agent migration.
- Normalized compatible reasoning aliases and filtered DeepSeek efforts unsupported by its native schema.
- Kept explicit reasoning toggles made in the editor when saving migrated models.

## 2.0.2 — 2026-08-22

### Fixed

- Replaced updates at the executable's current path, preserving user-renamed executable names.
- Deferred temporary `.apm-backup-*` cleanup to the restarted application with retries, so backup files do not remain after Windows releases the old process image.
- Added an explicit “Update and restart” action and automatic launch of the updated executable.

## 2.0.1 — 2026-08-22

### Added

- Published a Windows amd64 executable asset with an architecture-specific filename so the built-in updater can select it.
- Added `AGENTS.md` with versioning, build, release-asset, and updater workflow instructions.

### Fixed

- Corrected updater progress reporting so a check without a download does not report 100% completion.
- Preserved a verified staged update when checking the same latest release again.

## 2.0.0 — 2026-08-22

### Added

- Windows DPAPI-backed keychain storage with one-time migration of legacy reversible keychain data.
- Built-in model context/output limit presets and batch matching.
- ZCode, OpenCode, and DeepSeek Harness management, import, migration, backup, and restore workflows.
- Custom text-input context menu containing Cut, Copy, and Paste only.
- Bilingual project documentation and Windows release instructions.
- Optional fixed-source GitHub Release update checks, SHA-256 verified staging, and explicit restart confirmation.
- Options-page links for the author, project homepage, and GitHub issue reporting.

### Fixed

- Preserved target models when importing/merging DeepSeek and OpenCode providers.
- Preserved native OpenCode and DeepSeek provider fields during edits.
- Stopped writes when creating a safety backup fails.
- Completed the API-key keychain-to-import-page handoff.
- Improved narrow-window layout, touch scrolling, and keyboard focus visibility.

### Compatibility notes

- Windows-only release; Microsoft Edge WebView2 Runtime is required.
- Configuration files are reserialized when saved, so comments and formatting may change.
- This release does not include Linux or macOS binaries.
