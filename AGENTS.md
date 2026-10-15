# AgentProviderManager 贡献者指引

## 工作语言

- **全程使用中文工作**：与你（用户）的沟通、提交信息、代码注释、以及新增或修改的文档，一律使用中文。
- 例外仅限于必须与外部规范或工具保持一致的标识符与字面量：类型名、JSON/YAML 键、Git 仓库名、发布资产文件名，以及 Conventional Commits 的类型前缀（`feat` / `fix` 等）。

## 项目约束

- 本项目是仅支持 Windows 的 Wails 桌面应用。
- 构建依赖 Go 1.25+、Node.js/npm、Wails CLI v2.16.0，以及 Microsoft WebView2 运行时/SDK。
- 绝不读取、修改、打印或提交真实用户配置与凭据，尤其是 `C:\Users\<user>\.dsh`、provider 文件、备份、API Key、DPAPI 钥匙串文件或 GitHub token。
- 测试一律使用占位值。不要将 `node_modules`、前端生成物、临时可执行文件、`*.exe~` 或本地发布目录加入 Git。

## 版本变更

唯一真源是仓库根目录的 `VERSION` 文件。发版时只改它，然后运行分发脚本：

```text
node scripts/sync-version.mjs          # 把 VERSION 分发到所有消费点
node scripts/sync-version.mjs --check  # 仅校验（提交前）
```

脚本维护每一个派生位置：

- `internal/core/version_gen.go`（生成的 `core.AppVersion`，由 `main.go` 与 `GetAppInfo` 消费）
- `wails.json`（`info.productVersion` → Windows PE 版本信息）
- `frontend/package.json` 及其必须字节一致的 `frontend/package.json.md5`
- `frontend/package-lock.json`（根包与 lockfile 元数据）
- `README.md`（标题与 `AgentProviderManager-<version>-windows-amd64` 资产名）

前端不再硬编码版本：`AppSidebar.vue` 启动时从 `GetAppInfo` 读取。`go test ./...` 包含 `TestVersionSourcesInSync`，任何派生文件与 `VERSION` 漂移即失败；`CHANGELOG.md` 仍在发版时手工更新。

构建后用 `AgentProviderManager.exe --version` 验证可执行文件。

## 验证命令

在仓库根目录运行：

```text
cd frontend && npm ci && npm run build
cd ..
go test ./...
go vet ./...
go mod verify
wails build
git diff --check
```

Windows 更新器包另需编译其测试：

```text
GOOS=windows GOARCH=amd64 go test -c ./internal/update
```

## 后端约定

- 依赖方向严格单向：`main.go` → `internal/app` → `internal/core` → `internal/dshprovider` / `internal/zcodeprovider`。只有 `main.go` 与 `internal/app` 可以 import Wails；`internal/update` 不得依赖 Wails 层。
- 每次写配置都必须先走 `core.PrepareChange`（同级备份 + 还原点）。ZCode v2 的写入还要经过唯一收口 `WriteConfigBytes` → `guardZCodeV2Doc`；任何写入路径都不得绕过这两道门。
- 写入是原子的：同目录临时文件 + `Sync` + `Rename`。备份与临时文件后缀是固定词表：`.bak_<ts>`、`.snapshot_<ts>`、`.restore_*`、`.removed_<ts>`、`.apm-update-*`、`.apm-backup-*`。
- 并发修改用内容指纹（`mtime:size:sha256`）在 `a.mu` 下检测；不匹配即中止并提示重试。`internal/core/lock.go` 当前没有调用方。
- Wails 绑定方法绝不返回 `error`，一律返回 `{success, error, error_code}`；面向用户的字符串用 `core.ShortText`（300）截断。前端必须把 binding reject 与 `success: false` 两种情况都归一化为 rejection。
- 没有日志框架，也不用 `panic`。唯一的诊断输出是 `main.go` 里的 stderr 打印；用户可见的错误文案是中文。
- `context.Context` 不贯穿 `internal/core`：core 函数接收 `time.Duration`，由 app 层用 `context.WithTimeout` 包裹调用。
- 后端在 `init()` 里通过 `core.RegisterBackendFor` / `RegisterBackendForFormat` 自注册。包初始化阶段不得读取真实用户文件，格式探测函数必须无副作用（`internal/core/backend.go`）。
- 面向前端的 DTO 集中在 `internal/core/models.go`，使用显式 `snake_case` 标签。内部 meta 字段以下划线前缀（`_operation`、`_raw_*`）。空切片绝不序列化为 `null`。
- 不得移除的安全不变量：URL/SSRF 校验（默认拒绝 HTTP 与私网地址）、重定向逐跳复检并在跨主机跳转时剥离 `Authorization`、对每个对外错误做 `RedactSensitive`、以及更新器的主机白名单与防 DNS rebinding 复检。全仓库没有任何 `os/exec`。

## 测试

- 测试是同包 `*_test.go` 文件，尽量表驱动，辅助函数带 `t.Helper()`。
- 任何触盘测试都用 `t.TempDir()` 加 `t.Setenv("LOCALAPPDATA", ...)` / `USERPROFILE` 做沙箱，只用占位值，绝不写真实配置或还原点目录。
- 前端没有测试基建，依赖 `vue-tsc` 加真实 Wails 窗口的人工验证。源码中不保留 `TODO`/`FIXME` 标记——待办统一放进 `TODO.md`。

## 提交信息

遵循 Conventional Commits：类型前缀用英文（`feat`、`fix`、`refactor`、`build`、`docs`、`chore`，版本提交用 `release:`），描述正文用中文。scope 可选且少见。

## 发布资产

每个 Windows 发布都必须包含以下全部资产：

- `AgentProviderManager-<version>-windows-amd64.exe`
- `AgentProviderManager-<version>-windows-amd64.zip`
- `SHA256SUMS.txt`

裸架构 `.exe` 是内置更新器所必需的。只提供 ZIP 不够——更新器不解压归档，它直接挑选并暂存一个校验过的 Windows amd64 可执行文件。

ZIP 里面向用户的可执行文件名保持 `AgentProviderManager.exe`，这样用户重命名程序后，更新仍能替换同一路径。

## 更新器流程

更新器使用固定 GitHub 仓库 `Akiyuki1130/AgentProviderManager` 与 GitHub `latest` 发布端点。它只接受稳定 SemVer 发布，选择 Windows amd64 `.exe`，校验 GitHub SHA-256 摘要或 `SHA256SUMS.txt`，并把文件暂存到运行中可执行文件旁边。

下载绝不会替换或重启应用。安装需要用户显式确认。此后助手等待旧进程退出、再次校验暂存文件、备份当前可执行文件、替换原始路径（包括已被用户重命名的可执行文件）、启动失败时回滚，并在同一路径启动新可执行文件。重启后的应用会收到一个受限的清理指令，在 Windows 释放旧进程映像后重试删除临时 `.apm-backup-*` 文件。不得移除 URL/SSRF 校验、SHA-256 校验、no-shell 参数处理或重启确认。

## 前端 / UI 约定

- 前端工作遵循 `UI-GUIDELINES.md`（中文）。改动 `frontend/src/**` 之前先读它；其中是不得回退的硬性规则：零 emoji（图标来自 `@vicons/ionicons5`，经 `<n-icon>` 渲染，绝不写进 i18n 字符串）、零原生 `prompt()`/`confirm()`/`alert()`（改用 `stores/uiDialog.ts` 的 `uiConfirm`/`uiPrompt`）、不引入新的 UI 库、以及每个颜色/圆角/时长/显现都必须走 `styles/global.css` + `styles/theme.ts` 里的 token。
- 复用既有层，不要重造：`components/ui/PageHeader.vue`（所有页面）、`StickySaveBar.vue`、`UiDialogHost.vue`；composable `useToast` / `useStagger` / `useHighlight`；`global.css` 里的规范控件类 `.field` / `.switch` / `.check` / `.badge` / `.pill`。
- 前端改动后运行 `cd frontend && npm run build`（内含 `vue-tsc --noEmit`），并手工检查浅色与深色两套主题。
- 只用相对导入；`@/*` 别名虽已配置但零使用。跨组件共享状态用模块级 `ref`（见 `stores/uiDialog.ts`、`composables/useToast.ts`），不用 `provide`/`inject`。

## Git 与发布卫生

- 不要提交 API Key、`.dsh` 凭据、provider 配置、备份、DPAPI 数据、GitHub token、`node_modules`、生成的 `frontend/dist`、临时 EXE，或来自父目录的无关文件。
- 发布前检查发布目录，并核对 EXE、ZIP 与校验文件的 SHA-256。
- 使用正式、非草稿、非预发布的 GitHub 发布，并在上传后确认其为 `latest`。
- 漏洞通过 `SECURITY.md` 所述的私有渠道上报，且只用占位凭据复现——绝不建公开 issue。
- `reference/VrcFrameLimit` 是无关的本地克隆（已 gitignore）。它的文档与规则不适用于本项目。
