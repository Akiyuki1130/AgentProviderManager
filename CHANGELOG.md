# Changelog

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
