# ZCode Provider Manager (Go 重构版)

> 管理 **ZCode**（zcode.z.ai）第三方模型提供商（provider）配置的 Windows 桌面工具。
> Go 后端 + Vue 3 前端（Wails），UI 参照 [VrcFrameLimit](https://github.com/OneCat2015/VrcFrameLimit) 的 Fluent 风格重构。

原 Python 版：`ZCodeProviderManager-src/`（`core.py` + `main.py` + `ui/`）已完整保留在源码库中。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.24 + Wails v2 (WebView2) |
| 前端 | Vue 3 + Vite + Naive UI + Pinia + Vue Router + TypeScript |
| 加密 | Windows DPAPI（钥匙串） |

## 功能（与原版 1:1）

- **管理配置**：Provider 增删改查、ID 重命名、kind / baseURL / apiKey / options_json、模型批量编辑、重新获取模型列表
- **导入提供商**：输入 Base URL + API Key 拉取 `/v1/models`，智能推断 reasoning / token 长度，批量编辑后导入到 ZCode 配置
- **从 opencode.json 一键导入**：kind 推断、reasoning 档位翻译、合并/覆盖
- **合并其他 config.json**：来源为 ZCode 格式，原样保留仅归一化 kind
- **API Key 钥匙串**：本地 CRUD、DPAPI 加密、跨页签一键填入
- 安全写入：原子写 + 指纹校验 + 备份轮转（保留最近 2 份）+ 支持恢复、非本机强制 https、错误脱敏、私有/保留地址拦截

## 运行与构建

```bash
# 前端
cd frontend
npm install
npm run dev      # 开发（Vite）
npm run build    # 构建到 frontend/dist

# 后端（需 Go 1.24+）
go vet ./...
go build -o ZCodeProviderManager.exe .

# Wails 完整打包（需 wails CLI）
wails build
```

## 配置文件

- 目标：`%USERPROFILE%\.zcode\v2\config.json` 的 `provider` 对象
- 钥匙串：`%LOCALAPPDATA%\ZCodeProviderManager\keychain.json`
- 设置：`%LOCALAPPDATA%\ZCodeProviderManager\settings.json`（主题/语言）

## 目录

```
.
├── main.go                 # Wails 入口，embed frontend/dist
├── wails.json              # Wails 配置
├── frontend/               # Vue 前端（参照 VrcFrameLimit 布局）
│   ├── src/
│   │   ├── App.vue         # Fluent 侧边栏 + 顶部栏 + 路由视口 + Toast
│   │   ├── layout/         # AppSidebar / AppTopbar
│   │   ├── pages/          # Manage / Import / Opencode / Merge / Keychain
│   │   ├── components/     # ModelCard / ToastContainer
│   │   ├── stores/         # app / setting (Pinia)
│   │   ├── api/            # Wails 绑定封装
│   │   ├── styles/         # global.css / theme.ts (Fluent tokens)
│   │   └── router/
│   └── dist/               # 前端构建产物（被 Go embed）
├── internal/
│   ├── core/               # 核心逻辑（由 core.py 移植）
│   │   ├── constants.go / infer.go / urlutil.go / tokens.go
│   │   ├── cards.go / provider.go / config.go / fetch.go
│   │   ├── opencode.go / keychain.go / lock.go / paths.go
│   │   └── errors.go / models.go
│   └── app/                # Wails 绑定层（暴露给前端）
│       └── app.go
├── build/windows/          # 图标与 manifest
└── ZCodeProviderManager-src/  # 原 Python 版源码（保留）
```

## 安全说明

- 仅 `http/https` 的 outgoing 请求；发请求前校验 host，拒绝 localhost、环回、私有和保留地址。
- 非本机地址强制 `https://`。
- 私有/保留地址拦截在 `FetchModelsRaw` 与重定向链路中双重校验。
