# Changelog

## Unreleased

### Added

- **Two-stage connection probe (`🔌 测试连接`).** `core.TestConnection` first reuses the hardened model-fetch path (SSRF gates, redirect posture, error codes) against `/v1/models`, then sends a 1-token chat request to the kind's real endpoint (`/chat/completions` / `/messages` with `x-api-key` + `anthropic-version` / `/responses`) using the first discovered model, so authentication is proven against the endpoint that actually serves traffic. Responses report per-stage ok/latency/count with the existing `ModelFetchError` code taxonomy (DNS `CONN`, `TIMEOUT`, `HTTP_401`/`HTTP_404`, `BAD_URL`), and all messages pass through `RedactSensitive`. Exposed as `TestConnectionRaw` (form values) and `TestProviderConnection` (stored provider through the same backend credential resolution as `RefreshProviderModels`, so DSH `apiKeyEnv` secrets resolve transparently); wired into the provider editor and the import page with an inline ok/warn/err result line.
- **Keychain ↔ provider linkage.** API-key fields gained `🔑 从钥匙串` (searchable picker modal over DPAPI-protected entries that fills the field) and `📥 存入钥匙串` (saves the current key as a named entry). The chosen association is recorded app-side in `settings.json` (`keychain_links`, `agent|providerID` → entry id — ZCode's strict v2 schema forbids indirection inside `provider_config.json`, so harness configs keep the resolved value), surfacing as a sync chip under the field (覆盖 / 更新钥匙串 / 解除) and a `🔗 N 提供商` badge on keychain rows. Bindings: `GetKeychainLinks`, `SetKeychainLink`, `RemoveKeychainLink`.
- **Portable backup export + import (`apm-backup v1`).** The restore-points page now exports the current agent's providers (all or selected) to a JSON envelope and imports a backup back through the existing `ImportProvider` path, so every imported provider automatically gets its own backup + restore point. Key inclusion and encryption are orthogonal choices: without keys (default, share-safe — `StripBackupKeys` clears `APIKey`, `apiKey` inside `options_json` and the raw row) or with keys; the payload can be sealed as 明文 (plain — exports with keys require an explicit "export anyway" confirmation dialog), 默认加密 (sealed: AES-256-GCM keyed by SHA-256 of an app-embedded seed, decryptable by this app on any device, which keeps the file from being read as plain text but is not protection against someone extracting the seed), or 密码加密 (stdlib `crypto/pbkdf2` SHA-256, 600k iterations + AES-256-GCM; metadata stays locked until unlocked — `InspectBackup` returns `needs_password` and wrong passwords fail on the GCM tag). Overwriting an existing provider with a keyless backup keeps its local API key (matching the `MergeConfig` protection) and non-sensitive `options_json` such as custom headers round-trips through import. Files are written 0600; passwords are never persisted.
- Provider sidebar filtering and sorting on the manage page: a search box matching name/id plus a sort select (default order / 名称 / 模型数), with the header count switching to `filtered / total`.

### Fixed

- `go test` no longer pollutes the real restore-points directory: `TestZCodeToDeepSeek...`, `TestDeepSeekToZCode...` and the three `ApplyLegacyImport` tests in `internal/core` wrote their migration/import snapshots into `%LOCALAPPDATA%\AgentProviderManager\restorepoints` because they sandboxed only `DSH_HOME`. They now pin `LOCALAPPDATA` (and `USERPROFILE` + a cleared `ZCODE_PERSONAL_PROVIDER_CONFIG_FILE` for the reverse migration's target-store listing) into `t.TempDir()`, matching `setupMigrateTest`.

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
