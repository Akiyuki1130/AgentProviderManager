# AgentProviderManager contributor instructions

## Project constraints

- This is a Windows-only Wails desktop application.
- Build with Go 1.25+, Node.js/npm, Wails CLI v2.15.0, and the Microsoft WebView2 runtime/SDK.
- Never read, modify, print, or commit real user configuration or credentials, especially `C:\\Users\\<user>\\.dsh`, provider files, backups, API keys, DPAPI keychain files, or GitHub tokens.
- Use placeholder values in tests. Do not add `node_modules`, generated frontend output, temporary executables, `*.exe~`, or local release directories to Git.

## Version changes

When publishing a release, synchronize the version in:

- `main.go`
- `internal/core/constants.go`
- `wails.json`
- `frontend/package.json`
- `frontend/package-lock.json` (root package and lockfile metadata)
- `frontend/package.json.md5`
- `frontend/src/layout/AppSidebar.vue`
- `README.md` and `CHANGELOG.md`

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

Downloading never replaces or restarts the application. Installation requires an explicit user confirmation. The helper then waits for the old process to exit, verifies the staged file again, backs up the current executable, replaces the original path, rolls back on failure, and starts the new executable at the same path. Do not remove URL/SSRF validation, SHA-256 verification, the no-shell argument handling, or the restart confirmation.

## Git and release hygiene

- Do not commit API keys, `.dsh` credentials, provider configurations, backups, DPAPI data, GitHub tokens, `node_modules`, generated `frontend/dist`, temporary EXEs, or unrelated files from the parent directory.
- Before publishing, inspect the release directory and verify SHA-256 values for the EXE, ZIP, and checksum file.
- Use a formal, non-draft, non-prerelease GitHub release and verify that it is `latest` after upload.
