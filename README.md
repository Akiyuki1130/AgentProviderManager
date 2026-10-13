# Agent Provider Manager 2.0.8

> 一个面向 Windows 的多 Agent 模型提供商管理器：用一个原生桌面界面管理 ZCode 与 DeepSeek Harness 的 provider、模型、API Key、备份和迁移。（OpenCode 入口已在应用内隐藏，暂不适配。）
>
> **关键词 / Keywords:** Agent Provider Manager, ZCode, DeepSeek Harness, CCSwitch alternative, Claude Code provider manager, AI provider manager, OpenAI compatible, Anthropic, model catalog, API key keychain, Wails, Vue 3, Windows desktop.

[English version](#english-version)

## 中文说明

### 项目定位

Agent Provider Manager（简称 **APM**）是一个 Windows 桌面工具，用于在不同 Agent 工具之间管理第三方模型服务商配置。它直接读写用户选择的配置文件，并提供 provider 列表、模型卡片、批量编辑、模型自动匹配、导入、迁移、备份恢复和 API Key 钥匙串等能力。

它适合需要频繁切换模型供应商、同时使用多个 coding agent、或希望用图形界面维护 OpenAI-compatible / Anthropic / Responses 服务的用户。它不是代理服务器、模型网关或云端密钥托管服务。

### 支持的 Agent 工具

| Agent | 默认配置位置 | 支持的操作 |
|---|---|---|
| **ZCode** | `%USERPROFILE%\.zcode\v2\provider_config.json`（新版，优先）/ `%USERPROFILE%\.zcode\v2\config.json`（旧版） | 读取、编辑、保存、还原点、provider/model 管理、旧版供应商一次性导入、ZCode 3.14 兼容性检查与一键修复、跨 Agent 迁移（源 / 目标） |
| **DeepSeek Harness** | 0.2：`%USERPROFILE%\\.dsh\\profiles\\desktop\\cordis.patch.yml`（桌面端，优先）/ `profiles\\web\\cordis.patch.yml`（`dsh web`、CLI）/ `cordis.patch.yml`（home 级，优先级最高）。0.1：`settings.yaml`（`.yml` / `.json`，只作为旧格式保留） | 读取、编辑、保存、还原点、provider/model 管理、跨 Agent 迁移（源 / 目标）；provider 写入 `llm-pi-ai` 条目的 `config.providers`，API Key 以 `apiKeyEnv` 引用写入 `.credentials.yaml`（`version: 1` + `refs`） |

> **OpenCode**：入口已在应用内完全隐藏（暂不适配，后续再说）。相关代码与历史还原点数据保留，但顶部栏、迁移页与编辑器里都不会再出现 OpenCode。

DeepSeek Harness 0.2（当前桌面端与 CLI 的 web profile）把用户配置从单个 `settings.yaml` 改成 profile patch 层：桌面端是 `~/.dsh/profiles/desktop/cordis.patch.yml`，web（`dsh web`，等价于 `dsh --profile web`）是 `~/.dsh/profiles/web/cordis.patch.yml`，自定义 profile 是 `profiles/<name>/cordis.patch.yml`；`~/.dsh/cordis.patch.yml` 是 home 级 patch，优先级最高，若它定义了 `llm-pi-ai`，程序会拒绝写被它覆盖的 profile 层并说明原因。provider 写在 patch 里 `id: llm-pi-ai` 条目的 `config.providers` 上，API Key 只写引用（`apiKeyEnv`），密钥落到 `~/.dsh/.credentials.yaml` 的 `version: 1` + `refs` 布局（0.1 的扁平布局会在写入时升级，`records` 与注释保留）。未改动的 patch 行（含 `!!js` 表达式与注释）在保存时原样保留；0.1 的 `settings.yaml` / `settings.yml` / `settings.json` 仍可读写，但 DSH 0.2 只会导入它一次。ZCode 新版文件的位置可用 `ZCODE_PERSONAL_PROVIDER_CONFIG_FILE` 覆盖，程序会按文件内容判定当前目标是旧版（`config.json`）还是新版（`provider_config.json`）。顶部栏也可以选择已探测路径或通过文件选择器指定配置文件。保存时会重新序列化 JSON/YAML，因此不保证保留原文件的注释、缩进和键顺序。ZCode 新版（`provider_config.json`，`schemaVersion` 1）的读写规则已与 ZCode 3.14 对齐：只写 ZCode 接受的键与取值，并按 ZCode 的键序输出，避免它读到后又把文件重写一遍；如果文件里存在 ZCode 会拒绝的内容（例如个人规则里的 `builtinModelIds`、家族 `group`、旧式 `logo`），程序会把它列出来并提供一键修复（修复前照例建立还原点）。

> 当前程序定位为 **Windows-only**：使用 Wails + WebView2，Windows 钥匙串使用 Windows DPAPI。非 Windows 平台没有完整的产品支持承诺。

### 支持的 provider 类型和功能

- `openai-compatible`：OpenAI-compatible Chat Completions 服务。
- `anthropic`：Anthropic Messages 风格服务。
- `responses`：OpenAI Responses 风格服务。
- 创建、编辑、删除和重命名 provider。
- 添加、删除、搜索和批量编辑模型卡片。
- 编辑 reasoning、variants、context window、output token、modalities、headers 和额外 options。
- 通过 `/v1/models` 获取模型列表；Base URL 会规范化后再请求，响应仅支持 JSON。
- 常用模型上下文长度和输出长度自动匹配，可在导入页或管理页批量执行。
- API Key 钥匙串：Windows 使用 DPAPI 加密；旧版本可逆 Base64 数据只用于一次性迁移读取，新的保存不会再静默降级为 Base64。
- 还原点：每次修改配置前自动保存改动前的文件原文（逐字节保留注释与键序），并记录 Agent、绝对路径、格式、文件当时是否存在、大小、SHA-256、操作类型等元数据；“还原点”页可任选时间点回滚，回滚前会先给当前状态再建一个还原点，因此回滚本身也能再回滚；单个目标保留最近 50 个、合计不超过 64 MB。
- 同级 `.bak_<时间戳>` 备份、指纹检查与“恢复最近备份”；默认每个文件最多保留 2 份同级备份。
- 在 ZCode（新版 / 旧版）与 DeepSeek Harness（0.2 profile patch / 0.1 `settings.yaml`）之间选择性迁移 provider（追加/合并或覆盖），或从 `/v1/models` 获取模型后导入当前 Agent。
- 输入框提供只含“剪切 / 复制 / 粘贴”的自定义右键菜单。
- 不包含模型代理、计费、遥测或自动上传；可选的自动更新只从本项目 GitHub Release 检查并下载正式版本。

### 安全与隐私边界

1. 模型请求只允许 `http` / `https`。默认仅允许 HTTPS；用户显式开启“允许 HTTP”后，外部 HTTP 地址才会被允许。HTTP 会以明文传输 API Key 和请求内容，不建议在生产环境开启。
2. 发请求前及重定向过程中会校验 host，默认拒绝 localhost、环回、私有、链路本地、未指定和其他保留地址；用户显式开启“允许本地与内网地址”后才会放行（例如访问本机模型服务）。此校验是 SSRF 防护，不等同于完整的网络隔离。
3. 点击获取模型时，API Key 会通过 `Authorization: Bearer ...` 发送到用户输入的 Base URL；服务商可能记录请求、来源 IP、模型列表和认证信息。项目本身不会把模型配置或凭据上传到项目方，也没有遥测；可选自动更新只访问固定的 GitHub Release 地址。
4. DeepSeek 的 `.credentials.yaml`、provider 配置、同级 `.bak_` 备份、恢复快照与还原点内容都可能包含 API Key。还原点只写在本机 `%LOCALAPPDATA%\AgentProviderManager\restorepoints`（文件权限 0600），不上传、不写日志，但仍是明文副本：不要把这些文件提交 Git、上传工单或发送给他人，并可在“还原点”页按需删除或清理。
5. 导入页的临时流程可能使用前端 `sessionStorage` 传递 API Key；钥匙串页面的复制功能会把密钥放入系统剪贴板。使用后请清理剪贴板，避免剪贴板管理器、录屏和共享用户配置泄露密钥。
6. Windows DPAPI 绑定当前 Windows 用户环境。更换用户、迁移到另一台电脑或重装系统前，请先按业务需要迁移凭据；不要把 DPAPI 文件当作跨设备备份。
7. 配置写入采用临时文件、重命名、SHA-256 指纹检查、同级备份轮转与还原点，但不是数据库事务，也不能替代用户自己的离线备份。还原点默认每个目标保留最近 50 个、合计不超过 64 MB，超出后按最旧优先清理。

### 快速开始

#### 方式 A：下载 Release

1. 在 GitHub Releases 下载 `AgentProviderManager-2.0.8-windows-amd64.zip`。
2. 解压到用户有执行权限的目录。
3. 确认系统已安装 Microsoft Edge WebView2 Runtime；Wails 桌面窗口依赖 WebView2。
4. 启动 `AgentProviderManager.exe`。程序不会自动上传配置，也不会自动修改未选择的 Agent 文件。
5. 第一次保存前确认顶部栏中的 Agent、目标配置路径和备份状态。

#### 方式 B：从源码构建

需要 Go 1.25+、Node.js 18+、npm、固定版本的 Wails CLI，以及 Windows WebView2 SDK/runtime。

```powershell
# 在仓库根目录
cd frontend
npm ci
npm run build
cd ..
go test ./...
go vet ./...
wails build
```

前端 `dist` 和 Wails 生成目录是构建产物，不提交到 Git。干净检出后请先完成前端构建，再执行 Wails 打包。

### 使用教程

#### 1. 选择 Agent 和配置文件

启动后在顶部栏选择 ZCode 或 DeepSeek Harness。程序会显示探测到的路径、provider 数量和备份状态。若默认路径不正确，使用“选择配置”选择实际文件；DeepSeek 也可通过 `DSH_HOME` 指定目录。

在修改前建议先复制一份配置文件到离线位置。保存成功后程序会创建备份并报告备份路径；恢复操作会列出可用备份，选择正确时间点后再确认。

#### 2. 软件更新

在“选项”页可以手动检查 GitHub Release，也可以打开自动更新。自动更新默认关闭；开启后只会从固定的 `Akiyuki1130/AgentProviderManager` 正式 Release 检查并下载 Windows amd64 EXE。下载完成后程序不会突然退出，而是显示确认窗口；只有点击“更新并重启”并通过未保存更改确认后，更新助手才会等待旧进程退出、校验 SHA-256、替换当前 EXE 并用原文件名启动新版本。安装目录需要允许当前用户写入；失败时会尝试回滚。

#### 3. 管理 provider

在“管理”页选择 provider：

- 修改名称、ID、类型、Base URL、API Key 和 options JSON；
- 在模型区域添加、搜索或删除模型；
- 使用批量操作设置 reasoning、variants、上下文长度和输出长度；
- “自动匹配”只根据内置模型预设填充已识别模型，请在保存前检查结果；
- 重命名 provider 时确认下游 Agent 是否依赖原 ID。

程序会尽量保留未被界面编辑的原生 provider 字段（例如 ZCode 新版规则里本工具不编辑的键）；不同 Agent 的原生 schema 并不完全相同，重大升级前仍应保留备份。

#### 3. 从服务商获取模型

在“导入”页输入 Base URL、API Key、provider 名称和类型，点击“获取模型列表”。程序会规范化 Base URL 并访问对应的 `/v1/models` 端点，限制响应大小、模型数量和重定向次数，并对每次重定向再次执行地址安全检查。

服务商必须提供兼容的 JSON 模型列表接口；并非所有 Anthropic、Responses 或自定义网关都提供 `/v1/models`，拉取失败时请按照服务商文档填写模型。

#### 4. 使用 API Key 钥匙串

在“钥匙串”页添加名称、API Key 和备注。Windows 构建使用 DPAPI 保护钥匙串内容；配置文件和备份仍可能包含明文或环境变量引用，不能只保护钥匙串文件。

可以复制单个或多个 API Key，也可以使用“填入导入页”把一次性密钥传给导入流程。不要在共享屏幕、日志或截图中展示真实 API Key。

#### 5. 导入与迁移

- 在“迁移”页选择源 / 目标 Agent（ZCode 与 DeepSeek Harness 可互为源与目标）和各自的配置文件，预览两侧的 provider 列表后选择要迁移的条目：
  - ZCode 新版（`provider_config.json`）与旧版（`config.json`）、DeepSeek Harness 0.2 profile patch 与 0.1 `settings.yaml` 都通过各自的 config backend 读写，不再依赖旧版顶层 `provider` 映射，因此两个方向都可用。
  - 写入 DSH 0.2 profile patch 时 provider 落在 `llm-pi-ai` 条目，API Key 以 `apiKeyEnv` 引用写入 `.credentials.yaml`；被 home 级 patch 覆盖时会明确拒绝并提示直接编辑该文件。
  - “追加/合并”保留目标已有 provider 与模型，同 ID 模型以源为准；“覆盖”先清空目标 provider 列表，再写入所选条目。
- 在“导入”页输入 Base URL 与 API Key 后点击“获取模型列表”，勾选模型再导入当前 Agent；已存在的 provider 可选择合并模型或整体覆盖。
- 所有写入都会先建立同级备份与还原点，执行前请阅读预览并保留离线备份。

#### 6. 右键菜单和布局

文本 `input` 与 `textarea` 上右键只显示“剪切、复制、粘贴”。无选区时剪切和复制会禁用，只读输入框只允许复制；checkbox、按钮、文件选择器等非文本控件不会显示编辑菜单。菜单会在窗口边界内定位。

### 已知限制

- 仅提供 Windows 桌面版本，依赖 WebView2；目前没有 Linux/macOS 发布包。
- 没有内置前端 E2E 测试；发布前应在真实 Wails 窗口验证剪贴板、WebView2、DPAPI、配置备份和恢复。
- 大量模型会产生较多 DOM，2000 个模型接近后端上限时可能降低低配机器上的交互流畅度。
- 配置保存会重新序列化文件，注释和原始格式可能变化。
- 保存到 ZCode 新版配置会重写为规范结构：旧版专有、无法映射的字段不会写入；未知键仍然会被拒绝（ZCode 遇到未知键会把整份配置当空，因此宁可报错也不写出这样的文件）。该检查位于唯一的写盘入口，任何写入路径都无法绕过。ZCode 不认但本程序能读的兼容性问题会单独列出并提供一键修复。
- OpenCode 入口已在应用内完全隐藏（暂不适配）：`internal/core/opencode.go` 等代码与历史还原点数据保留，但顶部栏、迁移页、编辑器里都没有 OpenCode 入口。
- 跨 Agent 迁移不再被拒绝，但它会重写目标配置：ZCode 新版按严格 schema 重新编码，DSH 0.2 的 `llm-pi-ai` 条目会重新序列化（该条目内的注释可能变化），其余行、注释与 `!!js` 表达式原样保留。
- 被 home 级 patch（`~/.dsh/cordis.patch.yml`）覆盖的 `llm-pi-ai` 配置无法从 profile 层修改：程序会拒绝写入并提示，请直接编辑 home 级文件。
- 写入 profile patch 时只重写真正变化的节点：未被改动的 provider 子树连同其中的注释与 `!!js` 表达式原样保留；被修改的那个 provider 会重新序列化，其内部注释可能变化。
- 程序不会替用户判断第三方服务商是否可信；HTTP、API Key、备份和剪贴板风险由使用者承担。

### 开发与贡献

```text
main.go                         Wails 入口和版本
internal/core/                  配置、模型、URL 安全、迁移、备份、DPAPI
internal/dshprovider/         DeepSeek Harness 0.2 profile patch / 凭据文档
internal/app/                   Wails 绑定和应用状态
frontend/src/                   Vue 3 页面、组件、Pinia 状态和样式
build/windows/                  Windows manifest、图标和产品元数据
```

提交前至少运行：

```powershell
npm run build
go test ./...
go vet ./...
git diff --check
```

请不要提交真实的 `%USERPROFILE%` 配置、`.dsh` 凭据、API Key、备份、`frontend/dist`、`frontend/wailsjs` 或 `build/bin`。

### 许可证

本项目源码以 MIT License 发布，详见 [`LICENSE`](LICENSE)。第三方依赖分别遵循其各自许可证；发布二进制时请同时阅读依赖项目的许可和 WebView2 运行时条款。UI 使用 Fluent 风格设计语言，项目不声称拥有 Microsoft 商标或官方关联关系。

---

## English version

### What it is

Agent Provider Manager (**APM**) is a Windows desktop application for maintaining third-party model providers used by ZCode and DeepSeek Harness (the OpenCode entry point is hidden for now). It edits user-selected configuration files locally and offers provider/model editing, bulk model operations, model-limit presets, imports, migrations, backups, restore, a Windows DPAPI-backed API-key keychain, and a minimal edit context menu.

APM is a local configuration manager. It is not a proxy, model gateway, hosted secret vault, billing service, telemetry client, or mandatory updater. Optional updates use only this project's fixed GitHub Release source.

### Supported agent tools

| Agent | Default configuration | Supported operations |
|---|---|---|
| **ZCode** | `%USERPROFILE%\.zcode\v2\provider_config.json` (new format, preferred) / `%USERPROFILE%\.zcode\v2\config.json` (legacy) | Read/edit/save, restore points, provider/model management, one-time legacy provider import, ZCode 3.14 compatibility check and one-click repair, cross-agent migration (source / target) |
| **DeepSeek Harness** | 0.2: `%USERPROFILE%\\.dsh\\profiles\\desktop\\cordis.patch.yml` (desktop, preferred) / `profiles\\web\\cordis.patch.yml` (`dsh web`, CLI) / `cordis.patch.yml` (home level, highest precedence). 0.1: `settings.yaml` (`.yml` / `.json`, legacy only) | Read/edit/save, restore points, provider/model management, cross-agent migration (source / target); providers live in the `llm-pi-ai` entry's `config.providers`, API keys are stored as `apiKeyEnv` references in `.credentials.yaml` (`version: 1` + `refs`) |

> **OpenCode** is hidden from the application for now. Its code and historical restore-point data are kept, but it no longer appears in the top bar, the migration page, or the editor.

DeepSeek Harness 0.2 (today's desktop app and the CLI web profile) moved user configuration from a single `settings.yaml` to per-profile patch layers: the desktop app uses `~/.dsh/profiles/desktop/cordis.patch.yml`, `dsh web` (equivalent to `dsh --profile web`) uses `~/.dsh/profiles/web/cordis.patch.yml`, and a custom profile uses `profiles/<name>/cordis.patch.yml`; `~/.dsh/cordis.patch.yml` is the home-level patch and wins over every profile layer, so when it defines `llm-pi-ai` the app refuses to write the overridden profile patch and explains why. Providers live in the patch's `id: llm-pi-ai` entry under `config.providers`; API keys are written as `apiKeyEnv` references and the secret goes to `~/.dsh/.credentials.yaml` in the `version: 1` + `refs` layout (a 0.1 flat layout is upgraded on write, keeping `records` and comments). Untouched patch rows keep their `!!js` expressions and comments; 0.1 `settings.yaml` / `settings.yml` / `settings.json` remain readable and writable, but DSH 0.2 imports such a file only once. The harness home can be overridden with `DSH_HOME`. ZCode's new-format location can be overridden with `ZCODE_PERSONAL_PROVIDER_CONFIG_FILE`; the application decides from the file contents whether the target is the legacy (`config.json`) or the new (`provider_config.json`) format. You can also choose a configuration file from the application. Saving re-serializes JSON/YAML; comments, indentation, and original key order are not guaranteed to survive. The new ZCode format (`provider_config.json`, `schemaVersion` 1) is read and written according to the rules of ZCode 3.14: only keys and values ZCode accepts are written, in ZCode's own key order, so it does not rewrite the file after a save. If the file contains something ZCode rejects (for example `builtinModelIds` in a personal rule, a family `group`, or an old-style `logo`), the application lists it and offers a one-click repair that creates a restore point first.

The product is currently **Windows-only**. It uses Wails and WebView2, and the Windows keychain uses Windows DPAPI.

### Supported provider kinds and features

- `openai-compatible` for OpenAI-compatible Chat Completions services.
- `anthropic` for Anthropic Messages-style services.
- `responses` for OpenAI Responses-style services.
- Create, edit, delete, and rename providers.
- Add, delete, search, and bulk-edit model cards.
- Edit reasoning, variants, context window, output token limits, modalities, headers, and custom options.
- Discover models through a JSON `/v1/models` endpoint with response, model-count, and redirect limits.
- Apply built-in context/output presets to common models.
- Store API keys in a Windows DPAPI-protected keychain. Legacy reversible Base64 files are accepted only for one-time migration; new writes do not silently downgrade to Base64.
- Restore points: every change first stores the previous file byte-for-byte (comments and key order included) together with metadata (agent, absolute path, format, whether the file existed, size, SHA-256, operation, provider/model) under `%LOCALAPPDATA%\AgentProviderManager\restorepoints`. The **Restore Points** page can roll back to any of them; a rollback first snapshots the current state, so the rollback itself can be undone. 50 points per target path and 64 MB in total are kept.
- Sibling `.bak_<timestamp>` backups with fingerprint checks and "restore recent backup"; two sibling backups are kept per file.
- Migrate providers between ZCode (new / legacy) and DeepSeek Harness (0.2 profile patch / 0.1 `settings.yaml`), in merge or overwrite mode, or discover models through `/v1/models` and import them into the current agent.
- Show only Cut, Copy, and Paste in the custom context menu for text inputs.
- No proxying, telemetry, or background upload is included. Optional updates use the fixed GitHub Release source and always require explicit restart confirmation.

### Security and privacy boundary

- Only `http` and `https` URLs are accepted. HTTPS is the default. External HTTP is allowed only after the user explicitly enables the HTTP option, and it exposes API keys and request data in plaintext.
- Hosts are checked before requests and again across redirects. Localhost, loopback, private, link-local, unspecified, and other reserved addresses are rejected by default; they are only allowed after the user explicitly enables the "Allow local & private addresses" option (for example, to reach a local model server). This is SSRF protection, not a complete network isolation boundary.
- When the user requests model discovery, the API key is sent as `Authorization: Bearer ...` to the user-provided Base URL. The provider may log requests, source IPs, model information, and authentication data. APM does not upload data to the project owner and contains no telemetry or updater.
- DeepSeek credentials, provider files, sibling `.bak_` backups, restore snapshots and restore-point content may contain API keys. Restore points are written only locally under `%LOCALAPPDATA%\AgentProviderManager\restorepoints` (mode 0600) and are never uploaded or logged, but they are plaintext copies: never commit, upload, or share them, and delete or prune them from the Restore Points page when no longer needed.
- Optional updates use only HTTPS GitHub API/release hosts for this repository. Stable Windows amd64 assets are size-limited and SHA-256 checked before staging; installation waits for the current process to exit and requires an explicit restart confirmation. The updater does not accept arbitrary URLs or shell commands.
- The import flow may temporarily use browser `sessionStorage`, and copying a key places it in the system clipboard. Clear the clipboard and avoid screen sharing or shared browser profiles when handling secrets.
- DPAPI is tied to the current Windows user. Plan credential migration before moving to another account or machine.
- Atomic temporary-file replacement, SHA-256 fingerprints, sibling backup rotation, and restore points reduce accidental loss, but they do not provide a database transaction and do not replace your own offline backups. Restore points keep the 50 most recent entries per target path (64 MB in total) and prune the oldest first.

### Quick start

#### Download a Release

1. Download `AgentProviderManager-2.0.8-windows-amd64.zip` from GitHub Releases.
2. Extract it to a directory where you have execute permission.
3. Install Microsoft Edge WebView2 Runtime if it is not already present.
4. Run `AgentProviderManager.exe`.
5. Before the first save, verify the selected agent, target path, and backup status in the top bar.

#### Build from source

Requirements: Go 1.25+, Node.js 18+, npm, the pinned Wails CLI, and the Windows WebView2 runtime/SDK.

```powershell
cd frontend
npm ci
npm run build
cd ..
go test ./...
go vet ./...
wails build
```

`frontend/dist` and generated Wails bindings are build outputs and are intentionally not tracked. A clean checkout must build the frontend before running Wails packaging.

### User guide

1. Select an agent and verify its configuration path. For DeepSeek, use `DSH_HOME` or the file picker when needed.
2. Back up important files before the first edit. The application creates a backup on successful write and reports its path.
3. Edit providers and model cards in **Manage**. Review auto-filled model limits before saving.
4. Use **Import** to call a compatible JSON `/v1/models` endpoint. This is not a universal discovery API for every vendor.
5. Use **Keychain** for local key organization, but protect configuration files and backups as well. Clipboard and `sessionStorage` are sensitive surfaces.
6. Use **Migrate** only after reviewing the preview. ZCode (new / legacy) and DeepSeek Harness (0.2 profile patch / 0.1 `settings.yaml`) can be migrated into each other in both directions: providers are read through each side's config backend, written through the target's backend, and API keys land in the right place (`.credentials.yaml` for DSH 0.2). A profile patch shadowed by the home-level patch is still refused.
7. Restore from a listed backup only after checking its timestamp and target path.

### Known limitations

- Windows-only release; no Linux/macOS binaries are promised.
- Frontend behavior is not covered by a full E2E suite; validate the real Wails window before shipping.
- Very large model collections near the 2,000-model backend limit may be slower on low-end hardware.
- Saving reserializes configuration files and may change comments or formatting.
- Saving to the new ZCode format rewrites it in the canonical structure: legacy-only fields with no mapping are not written, and unknown keys are still rejected (ZCode treats a configuration containing unknown keys as empty, so the application refuses to write one). That check sits at the single byte-write entry point, so no write path can bypass it. Compatibility problems that this tool can read but ZCode rejects are listed separately with a one-click repair.
- OpenCode is hidden from the application for now: `internal/core/opencode.go` and the historical restore-point data are kept, but no UI entry point offers it.
- Cross-agent migration is no longer refused, but it rewrites the target: ZCode new-format files are re-encoded under the strict schema, and a DSH 0.2 `llm-pi-ai` entry is re-serialized (comments inside that entry may change) while every other row, comment, and `!!js` expression is preserved.
- An `llm-pi-ai` configuration in the home-level patch (`~/.dsh/cordis.patch.yml`) overrides the profile layer, so the profile patch cannot be edited from the app: the write is refused with an explanation, and the home file has to be edited directly.
- Writing a profile patch only rewrites the nodes that actually changed: untouched provider subtrees keep their comments and `!!js` expressions verbatim, while the provider that changed is re-serialized (comments inside it may change).
- Users remain responsible for provider trust, HTTP exposure, API-key handling, backup security, and clipboard hygiene.

### Development and license

Run `npm run build`, `go test ./...`, `go vet ./...`, and `git diff --check` before submitting changes. Never commit real user configuration, `.dsh` credentials, API keys, backups, generated frontend output, Wails bindings, or local binaries.

The source code is released under the MIT License; see [`LICENSE`](LICENSE). Third-party dependencies and the WebView2 runtime remain under their respective licenses. Fluent-style UI references do not imply Microsoft endorsement or affiliation.
