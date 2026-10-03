# Changelog

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
