# Changelog

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
