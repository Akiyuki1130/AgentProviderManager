# AgentProviderManager contributor instructions

## Project constraints

- This is a Windows-only Wails desktop application.
- Build with Go 1.25+, Node.js/npm, Wails CLI v2.16.0, and the Microsoft WebView2 runtime/SDK.
- Never read, modify, print, or commit real user configuration or credentials, especially `C:\\Users\\<user>\\.dsh`, provider files, backups, API keys, DPAPI keychain files, or GitHub tokens.
- Use placeholder values in tests. Do not add `node_modules`, generated frontend output, temporary executables, `*.exe~`, or local release directories to Git.

## Version changes

The single source of truth is the repository-root `VERSION` file. To publish a release, edit only that file and run the propagation script:

```text
node scripts/sync-version.mjs          # 把 VERSION 分发到所有消费点
node scripts/sync-version.mjs --check  # 仅校验（CI / 提交前）
```

The script maintains every derived location:

- `internal/core/version_gen.go` (generated `core.AppVersion`, consumed by `main.go` and `GetAppInfo`)
- `wails.json` (`info.productVersion` → Windows PE version info)
- `frontend/package.json` and its byte-exact `frontend/package.json.md5`
- `frontend/package-lock.json` (root package and lockfile metadata)
- `README.md` (title and the `AgentProviderManager-<version>-windows-amd64` asset name)

The frontend no longer hardcodes the version: `AppSidebar.vue` reads it at runtime from `GetAppInfo`. `go test ./...` includes `TestVersionSourcesInSync`, which fails if any derived file drifts from `VERSION`, and `CHANGELOG.md` is still updated by hand at release time.

Verify the executable with `AgentProviderManager.exe --version` after building.

## Validation commands

From the repository root:

```text
cd frontend && npm ci && npm run build
cd ..
go test ./...
go vet ./...
go mod verify
wails build
```

For the Windows updater package, also compile its tests with:

```text
GOOS=windows GOARCH=amd64 go test -c ./internal/update
```

## Release assets

Every Windows release must include all of these assets:

- `AgentProviderManager-<version>-windows-amd64.exe`
- `AgentProviderManager-<version>-windows-amd64.zip`
- `SHA256SUMS.txt`

The bare architecture-specific `.exe` is required by the built-in updater. A ZIP alone is not sufficient because the updater does not extract archives; it selects and stages a verified Windows amd64 executable directly.

Keep the ZIP's user-facing executable name as `AgentProviderManager.exe` so users can rename the application and still receive updates that replace the same path.

## Updater workflow

The updater uses the fixed GitHub repository `Akiyuki1130/AgentProviderManager` and the GitHub `latest` release endpoint. It accepts stable SemVer releases only, selects the Windows amd64 `.exe`, verifies the GitHub SHA-256 digest or `SHA256SUMS.txt`, and stages the file beside the running executable.

Downloading never replaces or restarts the application. Installation requires an explicit user confirmation. The helper then waits for the old process to exit, verifies the staged file again, backs up the current executable, replaces the original path (including a user-renamed executable), rolls back on launch failure, and starts the new executable at that same path. The restarted application receives a restricted cleanup directive and retries removal of the temporary `.apm-backup-*` file after Windows releases the old process image. Do not remove URL/SSRF validation, SHA-256 verification, the no-shell argument handling, or the restart confirmation.

## Frontend / UI conventions

- Frontend work follows `UI-GUIDELINES.md` (Chinese). Read it before touching `frontend/src/**`; it holds the binding rules that must not regress: zero emoji (icons come from `@vicons/ionicons5` via `<n-icon>`, never baked into i18n strings), zero native `prompt()`/`confirm()`/`alert()` (use `uiConfirm`/`uiPrompt` from `stores/uiDialog.ts`), no new UI library, and every color/radius/duration/reveal goes through the tokens in `styles/global.css` + `styles/theme.ts`.
- Reuse the existing layer instead of re-inventing it: `components/ui/PageHeader.vue` (all pages), `StickySaveBar.vue`, `UiDialogHost.vue`; composables `useToast` / `useStagger` / `useHighlight`; canonical control classes `.field` / `.switch` / `.check` / `.badge` / `.pill` in `global.css`.
- Run `cd frontend && npm run build` (includes `vue-tsc --noEmit`) after frontend changes, and check both light and dark themes by hand.

## Git and release hygiene

- Do not commit API keys, `.dsh` credentials, provider configurations, backups, DPAPI data, GitHub tokens, `node_modules`, generated `frontend/dist`, temporary EXEs, or unrelated files from the parent directory.
- Before publishing, inspect the release directory and verify SHA-256 values for the EXE, ZIP, and checksum file.
- Use a formal, non-draft, non-prerelease GitHub release and verify that it is `latest` after upload.
