# PI Agent（oh-my-pi）兼容设计与决策

- 文档状态：P1 已实现并交付（2026-11-10）；实现清单与验收记录见 `TODO.md` 的「已完成 — OMP」一节与 `CHANGELOG.md` 的 2.2.0 章节。本文档保留为设计与决策记录
- 适用基线：AgentProviderManager 2.2.0（设计基线 2.1.2）
- 目标客户端：PI Agent / oh-my-pi（CLI 名 `omp`）
- 本机验证版本：`omp/18.16.0`
- 调研时上游最新版本：`v18.18.6`
- 最后更新：2026-11-10（第二轮：全特性覆盖矩阵与 23 项缺口；同日 P1 落地）
- 关联文档：`MODEL-MANAGEMENT-PLAN.md`、`UI-GUIDELINES.md`、`AGENTS.md`

---

## 0. 摘要

AgentProviderManager（下称 APM）新增第三个可管理的 harness：**PI Agent（oh-my-pi）**。

核心结论：

1. omp 的 provider / model 定义在 `~/.omp/agent/models.yml`，不是 `config.yml`。
2. APM 只管理 `models.yml` 的 `providers` 子树，并在 P1 受控写入 `config.yml` 的 `modelRoles.default`。
3. 新增 `internal/ompprovider` 纯文档包与 `internal/core/backend_omp.go`，沿用 DSH 的 `yaml.Node` 节点级写入模式。
4. omp 的 schema 失败会**禁用全部自定义 provider**，且进程仍退出 0；因此写前校验是硬要求。
5. 不读取 `agent.db`、不执行 `!command`、不接管 OAuth、不要求管理员权限。

P1 交付后，用户可以在 APM 中：

- 切换 `PI Agent (oh-my-pi)` harness；
- 管理默认 profile（P1 通过已探测位置 / 文件选择器可切到命名 profile；P2 提供下拉）；
- 新建 / 编辑 / 删除 provider 与 model；
- 把模型设为 omp 默认模型（写 `modelRoles.default`）；
- 从 ZCode / DSH 迁移 provider，也可反向迁移；
- 使用 APM 的备份、还原点、钥匙串关联与连接探测；
- 只读查看 omp 原生协议 / `modelOverrides` / `discovery` 型 provider，不误写。

### 0.1 第二轮评审：全特性覆盖缺口（2026-11-10 复审）

第一版只覆盖了 provider/model 主线。按 APM 现有 77 个 Wails 绑定 + 6 个页面逐项复盘后，补齐以下缺口；标 **P0** 的会在实现前直接写坏或读错 omp 配置：

| # | 缺口 | 后果 | 处置 |
|---|---|---|---|
| G1 | 只有 `models.json` 时新建 `models.yml` | omp 优先读 `.yml`，旧 `.json` 永被忽略，用户既有 provider 全部失效 | §4.4 + §6 的 `.json`-only 规则：P1 拒绝写入并提示先跑一次 omp；P2 显式迁移 |
| G2 | 删除最后一个模型后 provider 只剩 `api` 等字段 | omp 把**整份** `models.yml` 判为加载失败，所有自定义 provider 被禁用 | §6 校验规则：空 models 的 provider 必须保留至少一个允许键 |
| G3 | `reasoning: true` 但无 `thinking` 时，误以为档位为空 | 读回丢档位；写回可能删掉 `reasoning` | §4.6 / §2.3：omp 有 4/5 档 fallback ladder |
| G4 | `apiKey` 引用语义写成“不透明字符串” | 用户填 `$KEY` 会被当字面量密钥发送；探测结果与 omp 不一致 | §2.3 / §4.5：只有 `!command` / 环境变量精确名 / 字面量三种 |
| G5 | 探测与拉取不发送 omp 的 `headers`，也不处理 `authHeader` / 自定义认证头 | 依赖自定义头的网关永远探测失败；与 omp 实际请求不一致 | §14.5：探测/拉取扩展 headers；按 wire api 的原生认证发；跨主机重定向剥离 |
| G6 | 默认模型下拉没有数据源 | `GetOMPModelRoles` 只有 roles，拿不到全部 `provider/model` | §5.2：同一绑定返回 `selectors` |
| G7 | `config.yml` 进入 locations 或 target | 恢复 `config.yml` 还原点会把当前 target 切到设置文件，随后保存 provider 会写坏它 | §4.4 / §7.2：`config.yml` 不可作为 provider 目标 |
| G8 | ZCode 专属绑定没有 agent 守卫 | omp 下仍会读 ZCode 文件，`RepairZCodeCompat` 还会生成 omp 标签的 ZCode 还原点 | §14.6：全部加 `currentAgent == zcode` 守卫 |
| G9 | `options_json` 键域未定义 | 用户可写入 omp 不认识或会被忽略的键 | §4.5：P1 只允许 `headers` / `authHeader` |
| G10 | 保存时仍可能拿 `_raw_provider` 当基座 | 源 agent 的私有结构污染 omp | §6 第 5 条：omp 的 `_raw_provider` 一律只读展示 |
| G11 | 引用保护只覆盖 config.yml，且只放在 Upsert | 删除 / 重命名仍可能留下悬空引用；`DeleteProvider`/`DeleteModel` 绕过检查 | §5.5：三个方法都检查；增加文件内模型引用 |
| G12 | 备份跨 agent 导出 / 导入未做私有结构隔离 | omp 的 `_raw_provider`、headers 泄漏进 ZCode/DSH | §7.4 |
| G13 | 前端批量层未适配 | 批量能力补全会给 omp 补上 video/audio/pdf、输出模态等无法表达的位 | §8 / §14.4 |
| G14 | 前端硬编码 kind / ID 正则 / `builtin:` 过滤 / 钥匙串备注 | 只读协议显示空白；omp ID 含 `.` 时自动保存永久暂停；备注写成 ZCode | §8 |
| G15 | i18n 只计划“加 key”，未处理现有硬编码中文 | omp 新功能中英混排；三语不齐 | §8 |
| G16 | 站点规则与协议映射缺失 | `FetchModels` 的 apiType 固定空串，omp 基本命中不到限定协议规则 | §14.3 |
| G17 | 迁移只写了单向 | omp → ZCode/DSH 未定义：不支持的 api、引用型 key 怎么办 | §7.3 |
| G18 | omp 首次运行前创建 `config.yml` | 会让 omp 跳过 `settings.json` → `config.yml` 的 legacy 迁移 | §5.3 |
| G19 | profile 支持口径自相矛盾 | §0 说 P1 支持任意 profile，P2 才做下拉 | §4.4：P1 = 文件选择器 / 已探测位置；P2 = 下拉 |
| G20 | `agent.db` 与 `models.yml` 的优先级未写清 | 用户改 `auth: oauth` 却发现密钥仍来自 models.yml，或反过来 | §2.6 |
| G21 | `modelPresets` DTO 结构写错 | 实现时按错误类型解析，导致只读保护失效 | §2.6：`{ modelRoles, defaultThinkingLevel? }` |
| G22 | `_vendor` / `_supported_levels` / `_capabilities_inferred` 未要求 omp store 填充 | 厂商分组恒 unknown、档位不弱化、批量补全退化 | §4.8 |
| G23 | 新操作类型与格式徽标未列全 | 默认模型写入的还原点显示原始 op；`omp` 格式徽标无样式 | §8 / §14.6 |

> 第二轮结论：23 项缺口**全部纳入 P1**（D12），预计 3–4 周；新增工作主要集中在 G1–G12 的后端兜底与前端的“按 agent 裁剪”。

---

## 1. 决策记录

| 编号 | 决策点 | 最终决定 | 状态 |
|---|---|---|---|
| D1 | P1 支持的 wire 协议 | 只支持 `openai-completions`、`openai-responses`、`anthropic-messages` | 已确认 |
| D2 | 默认模型（`config.yml` 的 `modelRoles`） | P1 支持设置；只管理 `modelRoles.default` | 已确认 |
| D3 | 删除 / 重命名被默认模型引用的对象 | P1 拒绝，并提示先切换默认模型 | 按推荐口径预置 |
| D4 | API Key 落盘方式 | 明文写入 `models.yml`，UI 与迁移页明确警告 | 已确认 |
| D5 | Provider 显示名 | omp 下显示 ID；名称输入只读；不写伪字段 | 已确认 |
| D6 | Agent ID | `omp`，显示名 `PI Agent (oh-my-pi)` | 按推荐口径预置 |
| D7 | P1 写入目标 | `models.yml` + `config.yml` 的 `modelRoles.default` | 已确认（由 D2 推导） |
| D8 | 其余 10 种 omp 原生协议 | P1 只读保护；P2 再做协议选择器 | 已确认（由 D1 推导） |
| D9 | 是否读取 `agent.db` | 永不读取 | 硬边界 |
| D10 | 是否执行 `!command` | 永不执行；按不透明字符串保留 | 硬边界 |
| D11 | 只有 `models.json` 时 | P1 只读可见 + 拒绝写入 + 提示先运行一次 omp；不做自动迁移 | 已确认（第二轮） |
| D12 | P1 范围 | 第二轮 23 项缺口全部纳入 P1 | 已确认（第二轮） |
| D13 | `anthropic-messages` 无 `thinking` 时的档位 | 只写 `reasoning: true`，不猜 mode；档位由 omp 推导 | 已确认（第二轮） |
| D14 | 新建 provider ID 与内置 ID 重名 | 允许并警告；按 omp 的 override 语义处理 | 已确认（第二轮） |
| D15 | 空 `apiKey` 的写入语义 | 未显式修改时保留目标已有 Key；显式清空才删除 | 安全默认 |
| D16 | 只读 provider 的删除边界 | 允许整体删除；禁止单模型删除与部分写入 | 安全默认 |
| D17 | `config.yml` 是否可成为 provider 目标 | 永不可以；只用于默认模型读写与引用保护 | 安全默认 |
| D18 | `modelRoles` / `modelPresets` 的 glob 引用 | 保守匹配；无法静态判定时默认拒绝删除 | 安全默认 |

> D11–D14 已在第二轮确认；D15–D18 按安全默认预置，如要调整只需改本文与实现。

---

## 2. 调研事实

### 2.1 本机安装

| 项目 | 事实 | 证据 |
|---|---|---|
| 版本 | `omp/18.16.0` | `omp --version` |
| 可执行文件 | `C:\AppData\Bun\bin\omp.exe`，约 8 KB Bun shim | 文件属性 |
| 实际包目录 | `C:\AppData\BunData\global\node_modules\@oh-my-pi\pi-coding-agent` | `omp.bunx` 指向 |
| 配置根目录 | `C:\Users\<user>\.omp` | `omp config path` |
| agent 目录 | `C:\Users\<user>\.omp\agent` | 同上 |
| 本机 `models.yml` | **不存在**；当前只有 `config.yml`、`agent.db`、`models.db` 等 | 目录只读列举 |
| 凭据存储 | `agent.db` 的 `auth_credentials` 表，JSON 明文 | `sqlite-credential-store.ts` |

本机没有现成的 `models.yml`，因此 APM 首次保存 omp provider 时会创建该文件。

### 2.2 路径与 profile

| 场景 | 路径 |
|---|---|
| 默认 profile | `%USERPROFILE%\.omp\agent\models.yml` |
| 命名 profile | `%USERPROFILE%\.omp\profiles\<name>\agent\models.yml` |
| 默认 profile 的 agent 目录覆盖 | `PI_CODING_AGENT_DIR` |
| 配置根目录名覆盖 | `PI_CONFIG_DIR`（默认 `.omp`） |
| 活跃 profile | `OMP_PROFILE` 优先；否则 `PI_PROFILE`；`OMP_PROFILE` 显式为空表示默认 profile |

补充规则：

- 命名 profile **忽略** `PI_CODING_AGENT_DIR`。
- `models.yaml` 是 `models.yml` 的兼容文件名；两者都存在时 `.yml` 优先。
- 只有 `models.json` 时，omp 会一次性迁移为 `models.yml`（**不删除** `.json`）；此后 `.json` 永远被忽略。
- 只有 `models.yaml` 且没有 `.yml` 时，直接读 `.yaml`，**不会**触发 `.json` 迁移。
- `config.yml` > `config.yaml`；`config.json` 完全不读、也不迁移。旧版 `settings.json` 只在 `config.yml` / `config.yaml` 都不存在时一次性迁移为 `config.yml`。
- 项目级 `<cwd>\.omp\config.yml` 只影响 settings，不定义 provider；项目级没有 `models.yml`。

文件优先级（源码 `config-file.ts:147-198`、`model-registry.ts:562`）：

| 目标 | 读取顺序 | APM 写入选择 |
|---|---|---|
| models | `.yml` > `.yaml` >（`.json` 仅迁移来源） | 写已存在的 `.yml` / `.yaml`；**只有 `.json` 时不写** |
| config | `.yml` > `.yaml` | 写已存在的 `.yml` / `.yaml`；都不存在时新建 `.yml` |

**G1（P0）**：如果用户只有 `models.json`，APM 一旦新建 `models.yml`，omp 就会改读这个新文件，原来的 `.json` 既不会被迁移也不会再生效，用户会以为自己的 provider 全部消失。P1 的处理见 §6.11。

**G18（P1）**：如果 `settings.json` 存在且 `config.yml` / `config.yaml` 都不存在，说明 omp 还没做过 legacy 迁移。此时新建 `config.yml` 会让 omp 永久跳过 `settings.json` 的迁移。P1 的“设置默认模型”必须拒绝并提示先运行一次 omp（§5.3）。

### 2.3 `models.yml` schema

顶层：

```yaml
providers:
  <provider-id>:
    ...
```

Provider 字段（P1 相关）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `baseUrl` | string | 定义 `models` 时必填 |
| `apiKey` | string | 见下方解析规则 |
| `api` | enum | 13 种 wire API 之一 |
| `headers` | map | 额外请求头 |
| `compat` | object | wire 级兼容开关 |
| `auth` | `apiKey` / `none` / `oauth` | 默认 `apiKey` |
| `authHeader` | bool | 是否注入 `Authorization: Bearer` |
| `discovery` | object | 动态发现模型 |
| `models` | array | 自定义模型列表 |
| `modelOverrides` | map | 只覆盖已有模型，不能新增 |
| `transport` | `pi-native` | auth-gateway 专用 |
| `remoteCompaction` | object | 远端压缩 |
| `guardrail*` / `requestMetadata` | — | Bedrock 专用 |

Model 字段（P1 相关）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 必填、非空 |
| `name` | string | 显示名 |
| `api` | enum | 模型级协议覆盖 |
| `kind` | enum | 默认 `chat`；`search` 不允许 |
| `baseUrl` | string | 模型级 endpoint 覆盖 |
| `reasoning` | bool | 是否推理模型 |
| `thinking` | object | 推理档位定义 |
| `input` | `("text" \| "image")[]` | 仅 text / image |
| `supportsTools` | bool | 工具调用 |
| `contextWindow` | number | 上下文窗口 |
| `maxContextWindow` | number | 扩展窗口，必须 `>= contextWindow` |
| `maxTokens` | number | 最大输出 |
| `cost` | object | 若提供，四个子字段都必需 |
| `headers` | map | 模型级请求头 |
| `compat` | object | 模型级兼容开关 |
| `tokenizer` | enum | 分词器 |
| `promptCache` | object | 缓存配置 |
| `contextPromotionTarget` / `compactionModel` | string | 模型引用 |

`thinking`：

```yaml
thinking:
  mode: effort | budget | google-level | anthropic-adaptive | anthropic-budget-effort
  efforts: [minimal, low, medium, high, xhigh, max]
  defaultLevel: medium
  effortMap: { ... }
  supportsDisplay: true
  requiresEffort: false
```

硬约束：

- `efforts` 只接受六档，不含 `off`。
- `thinking.mode` 在出现 `thinking` 时必填；`thinking` 还必须带 `efforts` / `levels` / `minLevel`+`maxLevel` 之一，否则 schema 失败。
- `input` 只接受 `text` / `image`。
- 未知 provider / model 键会被保留但不生效；未知 `compat` 键只产生诊断 warning。

**`apiKey` 解析规则（G4）**（源码 `resolve-config-value.ts:104-108`）：

1. 以 `!` 开头 → 作为 shell 命令执行，取 stdout；APM **永不执行**，只按不透明字符串保留。
2. 否则 → 把整个字符串当**环境变量名**做精确大小写查找；存在则取该变量值。
3. 环境变量不存在 → 整个字符串作为**字面量密钥**使用。

关键差异：

- **没有 `$VAR` / `${VAR}` 展开**。用户填 `$MY_KEY`，omp 会先找名为 `$MY_KEY` 的环境变量；通常找不到，于是把字面量 `$MY_KEY` 当密钥发出去。
- 环境变量存在但值为空时，回落到字面量名称。
- `headers` 的值也走同一套解析。

因此 APM 的 API Key 输入必须区分三种形态并给出提示（见 §4.5 的 `api_key_kind`），不能把引用串当普通密钥原样提交给探测。

**`thinking` 的默认档位（G3）**（源码 `pi-catalog/src/compat/resolve.ts:1018-1208`）：

- 模型写了 `reasoning: true` 但**没有** `thinking` 时，omp 不是“没有档位”，而是按层解析：
  1. 同 provider 内置参考模型带 thinking → 继承；
  2. identity / KDL 规则给出 ladder；
  3. 都没有 → 按 wire api 的 fallback ladder：
     - 默认：`minimal` / `low` / `medium` / `high` 四档；
     - `anthropic-messages`、OpenAI responses 系列、以及 `openai-completions` 且 `thinkingFormat: "openai"` + `supportsReasoningEffort`：再加上 `xhigh`，共五档；
     - **不会**默认给 `max`，也不是六档。
- `thinking: { efforts: [] }` 能通过 schema，但运行时按“无显式 thinking”回落到默认 ladder。
- 只有 compat 明确 `supportsReasoningEffort: false`、`trustExplicitThinkingOnly: true` 等少数情况才真的是空档位。

对 APM 的含义：读档位时不能因为 `thinking` 缺失就认为只有 `off`；写档位时也不能靠“省略 thinking”来表达空集合。

**空 models 的 provider 规则（G2）**（源码 `models-config.ts:52-88`）：

- `providers: {}` 或整个 `providers` 键缺失：合法。
- provider 的 `models` 缺失或 `[]`：schema 合法，但必须至少具备以下之一：`baseUrl`、`headers`、`compat`、`apiKey`、`auth: none`、`disableStrictTools`、`guardrailIdentifier`、`requestMetadata`、`remoteCompaction`、`modelOverrides`、`discovery`。
- 只有 `api` 不满足；空对象 `{}` 不合法。
- 一旦不满足，omp 不是跳过该 provider，而是**整份 models 配置加载失败**，所有自定义 provider 全部禁用。

### 2.4 omp 能表达的能力面

| APM 中立能力 | omp 落点 | 结论 |
|---|---|---|
| `input_modalities` | `input`（仅 text/image） | 可映射；video/audio/pdf 丢弃 |
| `output_modalities` | 无 | 不映射 |
| `tool_call` | `supportsTools` | 可映射 |
| `structured_output` | 无直接字段 | 不映射 |
| `native_web_search` | 无 per-model 字段 | 不映射 |
| `mid_conv_system` | 无直接字段 | 不映射 |
| `attachment` | 无 | 不映射 |
| `temperature` | 无 per-model 字段 | 不映射 |
| 推理档位 | `reasoning` + `thinking.efforts` | 可映射，但只有六档 |

### 2.5 真实沙箱验证

在 `%TEMP%` 下用占位密钥创建最小 `models.yml`，设置 `PI_CODING_AGENT_DIR` 后运行：

```text
omp models --json
```

有效配置返回：

```json
{"models":[{"provider":"apm-test","kind":"chat","id":"test-model",
"selector":"apm-test/test-model","name":"Test Model","contextWindow":128000,
"maxTokens":8192,"reasoning":true,"thinking":["low","medium","high"],
"input":["text","image"],"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0}}]}
```

删除 `apiKey` 后：

```text
{"models":[]}
Warning: models.yml validation failed — custom providers disabled
Failed to load config file models ... "apiKey" is required ...
EXIT=0
```

结论：

1. 本文设计的最小字段集能被 omp 18.16.0 正确读取。
2. schema 失败时 omp 仍退出 0，**不能依赖退出码判断成功**。
3. APM 必须在写盘前做本地严格校验。

验证只使用临时目录与占位密钥，未触碰真实 `~/.omp`；临时目录已清理。

### 2.6 认证、ID 与凭据优先级

| 事实 | 结论 | 证据 |
|---|---|---|
| `auth` 默认 | `apiKey` | `models-config.ts:211` |
| `auth: none` | 允许无 `apiKey`，进入 keyless 集合，请求不带认证 | `model-registry.ts:1848-1851` |
| `auth: oauth` | 允许无 `apiKey`；但**若同时写了 `apiKey`，它不会被忽略**，仍作为 config override 且优先于 OAuth | `model-registry.ts:1874-1876`、`pi-ai/src/auth/cascade.ts:403-406` |
| `authHeader` 默认 | 未设置（等价 false）；只有 `authHeader: true` 且存在 `apiKey` 时，config 层才追加 `Authorization: Bearer <key>` | `resolve-config-value.ts:176,193-199` |
| `agent.db` 优先级 | `runtime --api-key` → `models.yml` 的 `apiKey` → stored OAuth → `/login` 存下的 key → config fallback → 环境变量 → stored 静态 key | `pi-ai/src/auth/cascade.ts:379-479` |

**G20**：APM 改 `models.yml` 的 `apiKey` 一定生效，不会被 `agent.db` 覆盖。反过来，用户想改用 OAuth 时必须**删除** `apiKey`；只把 `auth` 改成 `oauth` 不会让已写的 key 失效。

**G5**：`authHeader: false` 只关闭 config 层的 Authorization 注入，不等于关闭底层 SDK 认证，也不阻止用户显式写的 `headers.Authorization`。APM 的探测必须携带 `headers`，并按 wire api 的原生认证发（OpenAI Bearer / Anthropic x-api-key）；`authHeader: true` 时再补 config 层 Bearer。否则和 omp 实际请求不一致。

**Provider ID（G14）**：

- `providers` 的键在 schema 上是开放字符串，没有字符集校验；`.`、空格都能通过。
- `/` 是选择器 `provider/modelId` 的分隔符（按第一个 `/` 切分），带 `/` 的 provider ID 基本无法被正常选择。
- 复用内置 provider ID 是合法的 override：同 `provider+id` 的模型替换内置条目，新 id 追加，**其余内置模型保留**，不是整 provider 替换。
- APM 的新建 ID 应限制为 `[a-zA-Z0-9._-]`，并禁止 `/` 与空白；与前端现有正则的差异见 §8。

`modelPresets` 的真实结构（G21）：

```yaml
modelPresets:
  <preset-name>:
    modelRoles:
      default: provider/model
      smol: provider/model
    defaultThinkingLevel: medium   # 可选
```

不是 `presets.<name>.modelRoles`，也不在 `models.yml` 里。

---

## 3. 兼容范围

### 3.1 做

- 识别并切换 `omp` harness。
- 读取 / 创建 / 修改 / 删除 `models.yml` 的 `providers`。
- 管理 `models[]` 的 ID、显示名、上下文、输出、推理档位、输入模态、工具调用。
- 保留并透传未知 provider / model 键、注释、`compat`、`cost`、`discovery`、`modelOverrides`。
- 默认 profile 与命名 profile 的路径探测、候选列表、文件选择。
- P1 设置 `config.yml` 的 `modelRoles.default`。
- ZCode / DSH → omp 的跨 Agent 迁移。
- omp 的备份、还原点、格式标签、还原校验。
- `config.yml` 中 `modelRoles` / `modelPresets` 的只读引用保护。

### 3.2 不做

- 不读取、不修改、不导出 `agent.db`。
- 不调用 `omp auth export/import`。
- 不接管 OAuth 登录或 token 刷新。
- 不执行 `models.yml` 中的 `!command`。
- 不写 `config.yml` 的完整 settings；P1 只写 `modelRoles.default`。
- 不修改 omp 安装目录、`natives`、`models.db`、sessions、logs。
- 不要求管理员权限；只写当前用户的 `%USERPROFILE%`。
- 不新增 `os/exec`；APM 运行时不启动 omp。
- 不把 APM 内部 `_` 前缀键写进 `models.yml`。
- `settings.json` 只判断存在性，不解析、不迁移。

### 3.3 权限与授权边界

| 维度 | 约定 |
|---|---|
| Windows 权限 | 以当前用户身份运行；不请求提权；`%USERPROFILE%` ACL 生效 |
| 文件写入 | 只写选中的 `models.yml` / `config.yml`；同级 `.bak_` 与 APM 还原点目录 |
| 凭据 | 只读写 `models.yml` 的 `apiKey`；不读 `agent.db`、DPAPI 密钥、OAuth token |
| 命令执行 | 不执行 `!command`、不启动 `omp`、不启动 shell |
| 网络 | 沿用 APM 现有的 URL/SSRF 校验；默认拒绝 HTTP 与私网地址，用户显式开启后放行 |
| 明文风险 | `models.yml` 无加密层；UI 与迁移页必须提示密钥明文落盘 |
| 备份 / 还原点 | `.bak_`、`.snapshot_`、APM 还原点内容都含明文 Key；沿用 `%LOCALAPPDATA%` 的用户 ACL，不额外加密；导出备份可选 plain / sealed / password |
| 自定义 headers | 可能含凭据；探测/拉取跨主机重定向时必须剥离，错误信息经 `RedactSensitive` |

---

## 4. 架构设计

### 4.1 Agent 身份

在 `internal/core/agents.go` 增加：

```go
AgentOMP AgentID = "omp"
```

显示名：

- `Label`: `PI Agent (oh-my-pi)`
- `LabelEn`: `PI Agent (oh-my-pi)`
- `DetectLabel`: `PI Agent 配置`

同步修改：

- `AllAgents()` 增加一项（顺序：ZCode、DeepSeek Harness、PI Agent）。
- `IsValidAgent()` 接受 `omp`。
- `NormalizeAgentID()` 接受 `omp`。
- `AgentDefaultPath()` 返回 `OMPDefaultPath()`。
- `AgentFormatName()` 返回实际文件名（`models.yml` / `models.yaml` / `models.json` / `config.yml` / `config.yaml`）。

### 4.2 新增 `internal/ompprovider` 包

职责：纯文档处理，不依赖 `internal/core`，不读真实用户文件。

建议文件：

```text
internal/ompprovider/format.go    // Format 枚举、Detect(path, data)
internal/ompprovider/paths.go     // OMPHome / profile / agent dir / config path
internal/ompprovider/load.go      // Load(data) -> *File
internal/ompprovider/sync.go      // yaml.Node 节点同步
internal/ompprovider/validate.go  // 写前校验
internal/ompprovider/config.go    // config.yml 的 modelRoles 读写
```

核心类型：

```go
type Format int

const (
    FormatUnknown Format = iota
    FormatModelsYAML
    FormatModelsJSON // 只读 / 导入源
    FormatConfigYAML
)

type File struct {
    root      *yaml.Node
    providers *yaml.Node
}

func Load(data []byte) (*File, error)
func (f *File) Providers() (map[string]interface{}, error)
func (f *File) SetProvider(id string, raw map[string]interface{}, mergeModels bool) error
func (f *File) DeleteProvider(id string) bool
func (f *File) DeleteModel(providerID, modelID string) (bool, error)
func (f *File) Validate() error
func (f *File) Bytes() ([]byte, error)
```

`config.yml` 侧：

```go
type ConfigFile struct {
    root *yaml.Node
}

func LoadConfig(data []byte) (*ConfigFile, error)
func (c *ConfigFile) ModelRoles() (map[string]string, error)
func (c *ConfigFile) SetModelRole(role, selector string) error
func (c *ConfigFile) Bytes() ([]byte, error)
```

### 4.3 后端与 Store

新增 `internal/core/backend_omp.go`、`internal/core/omp_store.go`、`internal/core/omp_models.go`。

```go
type ompBackend struct{}

func init() { RegisterBackendFor(string(AgentOMP), ompBackend{}) }

func (ompBackend) Descriptor() BackendDescriptor
func (ompBackend) Read(path string) (Doc, error)
func (ompBackend) ReadWithFingerprint(path string) (Doc, string, error)
func (ompBackend) Write(path string, doc Doc, expectedFingerprint string) error
func (ompBackend) Store(doc Doc) ProviderStore
```

`Doc` 内部键：

| 键 | 类型 | 用途 |
|---|---|---|
| `_omp_file` | `*ompprovider.File` | `models.yml` AST |
| `_omp_path` | string | 目标路径 |
| `_omp_model_roles` | `map[string]string` | `config.yml` 的角色引用 |
| `_omp_model_presets` | `map[string]OMPPreset`（`{modelRoles map[string]string, defaultThinkingLevel string}`） | 预设引用 |
| `_omp_config_err` | `error` | `config.yml` 读取/解析失败原因 |
| `_omp_needs_migration` | bool | 只有 `models.json`；`Read`/`Write`/`Upsert` 拒绝 |
| `_omp_file_name` | string | 实际 `.yml` / `.yaml`，写回同一文件 |

所有 `_` 键只在 `Doc` 内部使用，绝不落盘。

### 4.4 路径函数

| 函数 | 规则 |
|---|---|
| `OMPHome()` | `%USERPROFILE%\<PI_CONFIG_DIR 或 .omp>` |
| `OMPActiveProfile()` | `OMP_PROFILE` 已定义则用它；否则 `PI_PROFILE`；空串 = 默认 |
| `OMPAgentDir(profile)` | 默认 profile 且 `PI_CODING_AGENT_DIR` 非空 → 用它；命名 profile → `OMPHome()/profiles/<name>/agent`；否则 `OMPHome()/agent` |
| `OMPModelsPath(profile)` | 读取路径：已存在 `.yml` > `.yaml` > `.json` > 新文件 `.yml`；返回 `(path, needsMigration)`，`needsMigration=true` 表示当前只有 `.json` |
| `OMPModelsWritePath(profile)` | 写路径：已存在 `.yml` > `.yaml`；只有 `.json` 时返回 `.yml` 但标记不可写 |
| `OMPConfigPath(profile)` | 已存在 `config.yml` > `config.yaml` > 新文件 `config.yml` |
| `OMPDefaultPath()` | 活跃 profile 的 `OMPModelsPath().path` |
| `OMPProfiles()` | 枚举 `OMPHome()/profiles/*/agent` |

约束：

- 探测函数只读、无副作用。
- 不主动写 `models.json`。
- 只有 `models.yaml` 时写 `.yaml`，避免新建 `.yml` 遮蔽。
- 命名 profile 不使用 `PI_CODING_AGENT_DIR`。
- **G1**：只有 `models.json` 时（没有 `.yml` / `.yaml`），`OMPModelsPath` 返回 `.json` 并带 `needsMigration=true`；`Read` 允许**只读解析** `.json`（JSON 是 YAML 子集），以便用户仍能查看 / 导出 / 迁移现有 provider；`Write` / `Upsert` / `Delete` / `DeleteModel` 一律拒绝，前端顶部显示“请先运行一次 omp 完成 models.json 迁移”。不要在 P1 自动创建 `.yml`。
- **G7**：`config.yml` / `config.yaml` **不是** provider 目标。`OMPConfigPath` 只供“默认模型”读写与只读引用保护使用；`DetectConfigLocationsForAgent(omp)` 不得把它加入可选择的 location，`SetConfigPath` / `ChooseConfigFile` 必须拒绝它。否则 `RestoreRestorePoint` 会把 `targetConfigPath` 切到 `config.yml`，下一次 `SaveProvider` 会把设置文件按 providers 解析并写坏。
- **G19**：profile 支持分两阶段，避免与 P2 冲突：
  - P1：默认 profile + `PI_CODING_AGENT_DIR` 覆盖；命名 profile 通过 `ListConfigLocations` 已探测条目或文件选择器切换（复用现有 `SetConfigPath`）。不做独立下拉。
  - P2：在 Topbar 或 ManagePage 增加 profile 下拉，直接列出 `OMPProfiles()`，并把 `OMP_PROFILE` 的“当前活跃 profile”标出来。
- 路径候选顺序（`DetectConfigLocationsForAgent`）：
  1. 默认 profile：按其实际存在的文件列出（`.yml` 优先；`.yaml` 次之；只有 `.json` 时列出 `.json` 并标 `needs_migration`）
  2. `PI_CODING_AGENT_DIR` 覆盖目录（若设置且不同于默认）
  3. 每个命名 profile：同上，按 profile 名排序
  4. 不加入 `config.yml`；不加入项目级 `<cwd>\.omp`

### 4.5 Provider / Model 映射

#### Provider

| APM 概念 | omp 落点 | P1 行为 |
|---|---|---|
| Provider ID | `providers.<id>` 键 | 读写 |
| 显示名 | 无此字段 | 显示 ID；名称输入只读 |
| Base URL | `baseUrl` | 读写 |
| API Key | `apiKey` | 明文读写；已有引用（`!command` / 环境变量名）原样保留，不执行、不展开 |
| 类型 | `api` | 三种主流协议映射；其余只读 |
| 认证模式 | `auth` | `api_key_required=false` 且新建 → `auth: none`；已有 `auth: oauth` 时保持 oauth |
| 认证头 | `authHeader` | config 层的额外 `Authorization`；通过 `options_json` 保留/编辑。三种主流协议的探测按 SDK 原生认证（OpenAI Bearer / Anthropic x-api-key），不因 `false` 关闭原生认证 |
| 额外请求头 | `headers` | 通过 `options_json` 编辑；探测与拉取必须携带 |
| `compat` | `compat` | 只保留目标已有值；不从源迁移，也不由 APM 生成 |
| `discovery` | `discovery` | P1 保留，不自动生成；只读展示 |
| `modelOverrides` | `modelOverrides` | P1 保留，不展开 |
| 其它 | `transport`、`remoteCompaction`、`guardrail*`、`requestMetadata` | 原样保留 |

**`options_json` 的键域（G9）**：omp 下 `options_json` **只接受**以下两个键，其它键一律拒写并返回中文原因：

```json
{ "headers": { "X-Foo": "bar" }, "authHeader": false }
```

- `compat`、`discovery`、`modelOverrides`、`transport`、`remoteCompaction`、`guardrail*`、`requestMetadata` 在 P1 保留但不提供编辑入口（P2 再做专用编辑器）。
- 不允许通过 `options_json` 写入 `apiKey`、`baseUrl`、`api`、`models` 等已有专属字段，避免两套真源。
- **空 `options_json` 表示“不修改”**，不是“清空 headers”；只有显式传入 `{"headers":{}}` 才删除全部 headers（需二次确认）。
- `headers` 的值也可能是环境变量名 / `!command` 引用，APM 原样保存，不解析、不执行。

**`auth` / `api_key_required` 的读写口径**：

| 文件里的状态 | UI `api_key_required` | 保存行为 |
|---|---|---|
| 无 `auth`（默认 apiKey） | `true` | 写 `apiKey`；不写 `auth` |
| `auth: none` | `false` | 保持 `auth: none`；不写 `apiKey` |
| `auth: oauth` 且无 `apiKey` | `false` | 保持 `auth: oauth`；API Key 输入只读并标注“由 omp OAuth 管理” |
| `auth: oauth` 且有 `apiKey` | `false` | API Key 可编辑；保存时保留 `auth: oauth`，并提示“写了 apiKey 后 OAuth 不会生效”；清空 key = 删除 `apiKey`，OAuth 重新生效 |

模式切换：

- `api_key_required` 从 `true` 改为 `false`（默认 apiKey → none）：删除 `apiKey`，写 `auth: none`；保存前确认。
- `api_key_required` 从 `false` 改为 `true`（none → apiKey）：删除 `auth: none`，写 `apiKey`；Key 不能为空。
- `auth: oauth` 不通过布尔切换；用户要用 API Key 时保留 oauth 并写 key，或清空 key 回到 OAuth。
- 新建时 `api_key_required=false` 且填了 key：拒绝，提示“不需要 Key 的 provider 不应填写 Key”。

**空 Key 的写入语义**：`SaveProvider` 的 payload 需要携带内部标记 `_api_key_touched`（仅前端编辑器在用户实际修改/清空 Key 时置 true）：

- `touched=true` 且空 → 删除 `apiKey`；
- `touched=false` 且空 → 保留目标已有 `apiKey`（供备份导入 / 迁移 / 自动保存使用）。

否则“不含密钥的备份”或“自动保存时 Key 字段未加载”会静默清掉用户的 Key。该标记只在内存中使用，绝不落盘。

**协议映射（G16）**：

| APM kind | omp `api` | 站点规则 apiType |
|---|---|---|
| `anthropic` | `anthropic-messages` | `anthropic` |
| `responses` | `openai-responses` | `responses` |
| `openai-compatible` | `openai-completions` | `openai-compatible` |
| 其余（只读） | 12 种原生值 | 空串（fail-closed） |

`omp api → ZCode api.type` 的映射必须同时用于：`FetchModels` 的站点规则匹配、`ApplyModelPresetsForSite`、`RefreshProviderModels`。否则 omp 下所有“限定协议”的站点规则都会被跳过（现有实现传空 apiType）。

其余 omp `api` 值（`google-generative-ai`、`bedrock-converse-stream`、`azure-openai-responses` 等）在 P1 只读。

**provider 没有 `api` 时**：omp 允许每个 model 各自写 `api`。P1 的推导规则：

- 所有 model 的 `api` 一致且属于三种主流 → 用该值映射 kind；
- 混合协议或含不可映射值 → 整个 provider 只读，`raw_api` 显示“mixed”或首个未知值；
- 写回时不自动在 provider 级补 `api`，避免改变 omp 的模型级覆盖语义。

#### Model

| APM 卡片字段 | omp 落点 | 说明 |
|---|---|---|
| `model_id` | `models[].id` | 必填、非空 |
| `name` | `models[].name` | 与 ID 不同才写；清空则删除该键 |
| `context` | `contextWindow` | 正整数；清空则删除该键 |
| `output` | `maxTokens` | 正整数；清空则删除该键 |
| — | `maxContextWindow` | 保留旧值；若新 context 更大，抬到 `contextWindow` |
| `reasoning` + `variants` | `reasoning` + `thinking` | 见 4.6 |
| `capabilities.input_modalities` | `input` | 只保留 text/image 交集；空交集删除键；新建模型默认 text；不强制补 text（避免改掉 image-only / 未知模型的原意） |
| `capabilities.tool_call` | `supportsTools` | 只写 true；取消勾选 = 删除键（未知），不写 false |
| `headers` | `models[].headers` | 保留 |
| `_raw_cfg` 的其他键 | 同名 | 原样保留 |

不映射：

- `structured_output`
- `native_web_search`
- `mid_conv_system`
- `attachment`
- `temperature`
- `output_modalities`

实现约束：`cards.go` 的 `CfgToCardForSite` 输入形状是 ZCode/legacy 的模型节点（`limit.context`、`reasoning.*`、`variants`、`reasoningEfforts`、`modalities`）。omp 的 `models[]` 形状不同，**不要直接把 omp 节点喂给它**；由 `omp_store` 自行构造卡片，能力推断可以复用 `lookupCapabilityForSite`，但字段读取必须走 omp 自己的转换。

### 4.6 推理档位映射

omp 六档：

```text
minimal | low | medium | high | xhigh | max
```

读：

- `reasoning: false` 或缺失 → 关闭推理，`Variants = ["off"]`，`Reasoning = false`。
- `reasoning: true` 且 `thinking.efforts` 存在 → `Variants = ["off"] + efforts`（保持文件顺序，去重）。
- `reasoning: true` 且 `thinking` 缺失/无 efforts → **G3**：不要当成空档位。按 §2.3 的 fallback ladder 推断：
  - `anthropic-messages` / `openai-responses` / 支持 effort 的 `openai-completions`：`minimal,low,medium,high,xhigh`；
  - 其它：`minimal,low,medium,high`。
  - 推断值放进 `_supported_levels`，`Variants` 只放 `["off"] + 推断档位`；`_raw_cfg` 里仍保留“文件没有 thinking”的事实。
- `card.DefaultVariant = thinking.defaultLevel`，若不在 `Variants` 内则回落 `high`（没有 high 时取首个非 off）。
- `card.RawReasoningEnabled` 按 `reasoning` 设置；`card.OffAlias` 留空（omp 没有 off 拼法）。
- `_supported_levels` 永远只放六档中的非 off 值；`off` 由思考开关表达。

写：

- `card.Reasoning == false` 或只剩 `off`：
  - 删除 `thinking`；
  - 删除 `reasoning`（omp 默认 false）；
  - 不写 `false`。
- `card.Reasoning == true` 且有非 off 档：
  - 写 `reasoning: true`；
  - `efforts` 只写六档，按 `minimal→max` 规范顺序，过滤 `off` / `enabled` / 自定义档；
  - `defaultLevel` 必须落在 `efforts` 内，否则写首个或 `high`；
  - 已有 `thinking.mode` → 原样保留（这是用户的显式选择）；
  - 已有 `thinking.effortMap` / `supportsDisplay` / `requiresEffort` 原样保留；新增档位若 `effortMap` 没有对应键，omp 会用自己的默认映射，UI 只提示不阻断；
  - 没有 `thinking` 时，**P1 的 mode 策略**：

    | api | 无 thinking 时的写入 | 理由 |
    |---|---|---|
    | `openai-completions` | `mode: effort` + `efforts` | 兼容层能把 effort 映射到 wire |
    | `openai-responses` | `mode: effort` + `efforts` | 同上 |
    | `anthropic-messages` | **不写 `thinking`**，只写 `reasoning: true`；UI 标注“档位由 omp 自动推导” | Anthropic 需要 budget/adaptive 语义，P1 不猜 mode，避免写出不生效甚至报错的组合 |

  - P2 增加 `thinking.mode` 选择器（`effort` / `budget` / `google-level` / `anthropic-adaptive` / `anthropic-budget-effort`），届时 anthropic 也能精确写档位。
- 不得把 `card.Variants` 里的 `off` 写进 `efforts`；不得写 `thinking.efforts: []`（运行时会被当作无显式 thinking，等于丢掉用户选择）。

UI：

- omp 下隐藏“自填档位”输入。
- omp 下隐藏 `off` 胶囊，关闭推理用思考开关表达。
- 仍显示六档胶囊；`_supported_levels` 用于弱化不支持的档位。
- 批量“全强度”在 omp 下只补六档，不补 `off` / `enabled`。
- `anthropic-messages` 且文件原本没有 `thinking` 时，卡片显示一行提示：档位仅在 APM 中记录为展示值，实际由 omp 推导；P2 才可精确控制。

### 4.7 只读边界

`ProviderEdit` 增加：

```go
ReadOnly       bool   `json:"read_only,omitempty"`
ReadOnlyReason string `json:"read_only_reason,omitempty"`
APIKeyKind     string `json:"api_key_kind,omitempty"` // literal | env | command | none
RawAPI         string `json:"raw_api,omitempty"`      // omp 原始 api，只读展示
```

`ProviderSummary` 增加 `read_only`，让列表能先画只读徽标；`ConfigLocation` 增加 `needs_migration`，用于 `.json`-only 的条目标注。

以下情况只读：

| 场景 | 原因 |
|---|---|
| `api` 不是三种主流协议 | P1 不建模其余 omp 原生协议 |
| provider 没有 `baseUrl` | 例如只做 `modelOverrides` 的内置 provider 覆盖 |
| 只有 `modelOverrides` 没有 `models` | APM 不展开内置模型覆盖 |
| `auth: oauth` 且无 API Key | 可管理模型，但不接管 OAuth |
| 只有 `models.json` 且尚未迁移 | G1；不写新 `.yml` |
| provider 的 `models` 里出现 `kind: search` 等 APM 不建模的条目 | 避免保存时丢条目 |
| provider / model 节点是 YAML 别名（`*anchor`）或使用 `<<` merge | 改写会波及共享节点；P1 只读，P2 再做展开 |
| 文件使用多文档（`---`） | omp 不读多文档；直接拒绝加载 |

只读 provider 的规则：

- 允许整体删除（删除仍要做引用保护）。
- **不允许** `DeleteModel`、单模型编辑或部分写；只读 provider 只能整体删除。
- 禁止保存与自动保存；后端 `Upsert` / `SaveProvider` 必须兜底拒绝，不能只靠前端。
- `kind` 下拉必须禁用并显示 `raw_api`；不能让浏览器/Element 把未知值 coerce 成 `openai-compatible`。
- 探测按钮可以保留，但协议必须按 `raw_api` 映射；映射不到时直接返回“该协议 P1 不支持探测”，不要按 openai-compatible 误测。

### 4.8 后端必须填充的内部字段（G22）

`List` / `Get` 返回的卡片必须填：

| 字段 | 来源 | 作用 |
|---|---|---|
| `_vendor` | 现有 `detectVendor` / 能力表按 model id 推断 | 厂商分组与筛选；否则全部 unknown |
| `_supported_levels` | `thinking.efforts`，或 §2.3 的 fallback ladder | 档位弱化、批量补全 |
| `_capabilities_inferred` | 现有能力分层表，**过滤成 omp 可表达的位** | 展示来源与“按能力表补全” |
| `capabilities` | 文件里能明确读出的值（`supportsTools` / `input`） | 三态；未知不写 |

`_capabilities_inferred` 必须裁剪：omp 只表达 `input_modalities`（text/image）与 `tool_call`，不要给出 `output_modalities`、`structured_output`、`native_web_search`、`mid_conv_system`、`attachment`、`temperature` 的推断值。否则前端批量补全会补上无法落盘的位。

---

## 5. 默认模型与 `config.yml`

### 5.1 范围

P1 只管理：

```yaml
modelRoles:
  default: my-gateway/fast-chat
```

不管理：

- `smol`
- `slow`
- `advisor` / `memory` / `tiny` / `widget` / `task` 等其它角色
- `modelPresets.*.modelRoles`
- `defaultThinkingLevel`

> `plan` 是 omp 的会话模式，不是 `modelRoles` 的角色；第一版把它列进角色是错的。

`modelPresets` 与其余角色只做只读引用保护。

### 5.2 绑定

新增：

```go
GetOMPModelRoles() map[string]interface{}
SetOMPModelRole(role, selector string) map[string]interface{}
```

P1 的 `role` 白名单只允许 `"default"`；P2 可放开其他角色，不改前端协议。

返回示例：

```json
{
  "success": true,
  "path": "C:\\Users\\<user>\\.omp\\agent\\config.yml",
  "exists": true,
  "roles": { "default": "my-gateway/fast-chat" },
  "selectors": ["my-gateway/fast-chat", "my-gateway/deep", "other/vision"]
}
```

**G6**：`selectors` 是默认可下拉的全部 `provider/model`，必须由后端直接扫描 `models.yml` 生成。只返回 `roles` 时前端无法拼出选项（`ListProviders` 的 summary 不含模型 ID，逐个 `GetProvider` 代价太高）。`selectors` 的规则：

- 只列 `models[]` 里显式声明的模型；
- 只列 `api` 可映射到三种主流协议的 provider（只读 provider 的模型仍可作为“当前值”显示，但不建议新设）；
- 按 `provider` 再按 `model` 排序；
- 与 `modelRoles.default` 的当前值无关，当前值即使不在列表里也原样返回。

### 5.3 校验与写入顺序

`SetOMPModelRole("default", selector)`：

1. 当前 Agent 必须是 `omp`。
2. `role` 必须在白名单。
3. 读取当前 `models.yml`，确认 `provider/model` 选择器存在；空串表示**删除** `modelRoles.default` 键（不是写空字符串）；P1 只接受精确选择器，拒绝 glob（`*` / `?`）与 `@role` 别名（下拉本身只产生精确值）。
4. 读取 `config.yml` 并校验根节点是 mapping；解析失败则拒绝。
5. **G18**：若 `config.yml` / `config.yaml` 都不存在，但 `settings.json` 存在，拒绝并提示“请先运行一次 omp 完成旧版设置迁移”；否则 APM 新建的 `config.yml` 会让 omp 永久跳过迁移。
6. **G7**：`config.yml` 只能作为本绑定的读写目标，绝不能写入 `agentPaths` / `targetConfigPath`。
7. 对 `config.yml` 调用 `PrepareChange`（同级 `.bak_` + 还原点，格式 `omp-config`，操作 `set_default_model`）。
8. `WriteConfigBytes` 原子写入；指纹不匹配则中止。

注意：

- 设置默认模型**不修改** `models.yml`，因此不存在双文件同时写入。
- 如果用户在同一次操作中新建 provider 并设为默认，必须先保存 provider，再设置默认。
- 失败时 `models.yml` 不受影响；`config.yml` 可能留下备份与还原点，这是预期行为。

### 5.4 UI

在 `ManagePage.vue` 顶部增加“默认模型”卡片，仅 `omp` 显示：

- 下拉选项 = 后端返回的 `selectors`（不是前端自行扫描）。
- 当前值不在 `selectors` 中时，仍显示“当前值”，允许保留不改。
- 提供“清除默认模型”。
- 提示：写入 `config.yml`；项目级 `.omp/config.yml` 可能覆盖全局值。
- 未保存的新 provider/model 不出现在下拉中。

### 5.5 引用保护

删除 / 重命名时（G11）：

- 如果 provider 被 `modelRoles.*` 或 `modelPresets.*.modelRoles` 以 `<providerID>/` 引用 → 拒绝。
- 如果 model 被引用为 `providerID/modelID`（可带 `:effort`、`@revision` 等后缀）→ 拒绝。
- **文件内引用**：如果被删除/重命名的 model 出现在同 provider 的 `models[].compactionModel` 或 `models[].contextPromotionTarget` 中 → 拒绝，并指出引用它的模型 ID。
- **重命名**按“旧 ID 被删除 + 新 ID 新增”处理；只要旧 ID 被引用就拒绝，不允许静默留下悬空引用。
- 引用检查必须同时放在 `Upsert`、`Delete`、`DeleteModel` 三个 store 方法里。原因：前端删除模型只是从表单移除再保存（`Upsert`）；`DeleteProvider` / `DeleteModel` 绑定虽然当前前端未用，但备份导入、合并、脚本都可能调用。
- 引用解析规则：`providerID/modelID` 按**第一个** `/` 切分；`modelID` 内允许 `@`；`:effort` 只在最后一段且命中六档时剥离。provider ID 含 `/` 的条目在 APM 里标记为只读，避免解析歧义。
- **模式值**：`modelRoles` / `modelPresets` 的值可能是 glob（`*`、`?`）、`@role` 别名或带 `:level` 的模式，不是只有精确 `provider/model`。引用保护必须保守匹配：
  - 精确值：按上面的规则；
  - `provider/*` 或 `provider/*:level`：删除该 provider 下任何模型都算被引用；
  - `*/modelID`：删除任何 provider 下的同名模型都算被引用；
  - `@role` / `*`：无法静态判定，删除任何 provider/model 都提示“可能被模式引用，无法自动判断”，默认拒绝并要求用户先改 `config.yml`。
- 提示文案应引导用户先到“默认模型”切换到其它模型（`modelPresets` 在 P1 只读，用户需手动改 `config.yml`）；P1 不自动改 `config.yml`，避免双文件半更新。

保护范围只到“拒绝”：

- `modelRoles` / `modelPresets` 的其余角色（`smol`、`slow` 等）只读，不提供编辑。
- 删除只读 provider 时同样做引用检查。

### 5.6 限制

- APM 只能列出 `models.yml` 里的模型；不能把内置 provider 的模型设为默认，除非它已在 `models.yml` 中被覆盖声明。
- APM 写全局 `config.yml`；项目级 `.omp/config.yml` 可能覆盖。
- 不写 `defaultThinkingLevel`。

---

## 6. 写入安全

1. 每次写配置先 `core.PrepareChange`。
2. 写盘统一走 `WriteConfigBytes`（同目录临时文件 + Sync + Rename + 指纹校验）。
3. 写前校验：
   - 建议新增可选接口 `DocValidator`，在 `PrepareChange` 之前校验 Upsert 后的内存文档；
   - 避免校验失败仍留下无用备份/还原点。
4. 只写真值：
   - 未知能力位不写；
   - 不写 `false` / `null` / `enabled`；
   - 不写 `_apm_capabilities`、`_raw_cfg`、`_off_alias` 等内部键；
   - **注意**：现有 `cards.go` 的 `stripAPMInternalModelKeys` 只处理 map 形状的 models，omp 的 `models[]` 是数组，必须在 `ompprovider` 里逐项剥离；不能指望复用该函数。
5. **目标文档是唯一保留基座（G10）**：
   - 迁移时不得用源 `_raw_provider` / `card.RawCfg` 作为目标基座；
   - 源只贡献规范化后的 `base_url`、`api_key`、`kind`、cards；
   - 同 agent 的普通保存也一样：`Upsert` 只把已知字段套写到目标节点上，**`_raw_provider` 永远只用于展示**，不能整体 replace，否则源私有键、被裁剪的能力位、旧 `compat` 会重新流回文件。
6. `maxContextWindow`：
   - 保留旧值；
   - 若新 `contextWindow` 更大，抬到 `contextWindow`；
   - 否则 omp 校验失败。
7. `cost`：
   - 保留旧值；
   - 若提供，四个子字段必须齐全。
8. 未知键：
   - provider/model 级未知键保留；
   - 未知 `compat` 键只告警，不阻断。
9. 指纹冲突：
   - 目标被 omp 或其他程序改过 → 中止并提示重试。
10. 编码：
    - 读取去 UTF-8 BOM；
    - 写出 UTF-8 + LF；
    - YAML 2 空格缩进 + 末尾换行。
11. 目录：
    - 首次写入 `~/.omp/agent/models.yml` 或命名 profile 前，`MkdirAll` 目标目录（仅 `%USERPROFILE%` 下）；
    - 目录创建失败要返回明确错误，不能留下半写文件；
    - 不修改目录 ACL。
12. 未变化字段保留原节点：
    - `apiKey`、`headers` 的值可能是 `!command` / `!!js` 等带标签的标量；从 map 读出来再写回会丢标签或把标签变成普通字符串；
    - `Upsert` 必须按“值是否变化”决定写入：未变化的 `apiKey` / header 值直接保留原 `yaml.Node`；
    - 只有用户明确修改时才写新标量，并在 UI 提示“会从引用变成字面量”。

写前校验规则：

- provider ID：
  - 新建 / 重命名时只允许 `[A-Za-z0-9._-]`，禁止 `/` 与空白（选择器分隔符）；
  - 文件里已有的、不符合该规则的 ID 不拒绝读取，只标记为只读（避免用户无法删除历史条目）；
  - 与内置 provider ID 重名时允许保存，但返回 `builtin_override: true` 并在 UI 提示“会覆盖内置 provider 的同名模型，不会删除其余内置模型”。
- `models` 非空时：
  - `baseUrl` 必填；
  - `apiKey` 必填，除非 `auth: none` / `auth: oauth`；
  - `api` 必须在 provider 或每个 model 上。
- `models` 为空或缺失时（G2）：
  - provider 必须至少保留以下之一：`baseUrl`、`headers`、`compat`、`apiKey`、`auth: none`、`disableStrictTools`、`guardrailIdentifier`、`requestMetadata`、`remoteCompaction`、`modelOverrides`、`discovery`；
  - 禁止写出空对象 `{}` 或只有 `api` 的 provider；
  - 删除最后一个模型时，如果剩余字段不满足上述条件，必须同时删除整个 provider，或拒绝并提示先补 `baseUrl` / `apiKey`。
- model：
  - `id` 非空，同一 provider 内不得重复；
  - `contextWindow` / `maxTokens` 若提供必须 > 0；
  - `maxContextWindow >= contextWindow`；
  - `thinking` 出现时 `mode` 必填，且必须带 efforts / levels / minLevel+maxLevel；
  - `efforts` 只能是六档；
  - `defaultLevel` 应在 `efforts` 内；
  - `cost` 若提供，四字段齐全；
  - `kind` 与 `api` 匹配（显式 kind 时）；
  - `models[].api` / `baseUrl` 只能保留，不由 APM 生成。
- `apiKey` / `headers` 写值：
  - 以 `!` 开头的值只允许原样保留，不得由 APM 生成或执行；
  - 形如 `$VAR` / `${VAR}` 的值在保存时警告“omp 不会展开该写法，会当作字面量密钥”；不阻断保存，但 UI 必须提示；
  - `headers` 的键不能是空的，值允许引用串。
- `discovery`：
  - provider 级 `discovery` 且 `type != proxy` 时必须有 `api`；
  - `injectV1` 只允许 `openai-models-list`；
  - `advertisedReasoning` 只允许 `openai-models-list` / `lm-studio` / `proxy`。
- 文件级：
  - 根节点必须是 mapping；
  - `providers` 若存在必须是 mapping；
  - 不允许多文档 YAML（`---` 分隔）；
  - 重复键按 `yaml.v3` 的报错处理，不静默取最后一个。

---

## 7. 备份、还原与迁移

### 7.1 先修 YAML 还原 Bug（P0）

`RestoreBackup` 目前用 `LoadConfigForRestore(bakPath)` 校验备份，而 `isYAMLPath` 只看 `.yml` / `.yaml` 后缀。备份文件名是 `models.yml.bak_2026...`，后缀被 `.bak_*` 遮住，导致 YAML 备份被当成 JSON 解析。

这不是 omp 新引入的问题，DSH 的 YAML 备份同样受影响。

修复方式：

- `LoadConfigForRestore(target, bakPath)` 按 `target` 的扩展名选择解析器；或
- 校验前剥离 `.bak_<ts>` / `.snapshot_*` / `.restore_*` 后缀。

必须补 DSH + omp 两条 YAML 还原测试。

### 7.2 还原点格式

新增：

```go
FormatOMP       = "omp"          // models.yml
FormatOMPConfig = "omp-config"   // config.yml
```

- `ConfigFormatAt`：
  - `AgentOMP` + `models.yml/.yaml/.json` → `FormatOMP`
  - `AgentOMP` + `config.yml/.yaml` → `FormatOMPConfig`
- `validateRestoreContent`：
  - 两者都只做 YAML 语法 + 根节点 mapping 校验；
  - 不套用 APM 的严格 schema，避免 omp 未来新增合法键被误判。
- 前端 `RestorePointFormat` 增加 `'omp'` / `'omp-config'`。
- `formatLabel` 三语：
  - `PI Agent (models.yml)`
  - `PI Agent (config.yml)`
- `RestorePointPage.loadCompat()` 限制为 `agent === 'zcode'`。
- 操作类型新增 `set_default_model`，前端 `KNOWN_OPS` 与三语文案同步。

**G7（P0）**：`RestoreRestorePoint` 目前只要目标路径属于当前 agent 的已知位置，就会把 `targetConfigPath` 切过去。`config.yml` 绝不能进入 omp 的可管理位置，否则：

1. 恢复一次 `config.yml` 还原点 → 当前目标变成设置文件；
2. 下一次 `ListProviders` 会把 `config.yml` 当 providers 文件解析；
3. `SaveProvider` 会按 provider 结构重写 `config.yml`，设置被写坏。

因此：

- `config.yml` 的还原点只允许“写回内容”，不允许切换 `targetConfigPath`；
- 恢复完成后 `current_target` 仍必须是 `models.yml`；
- `ValidateRestoreContent` 对 `omp-config` 只做 YAML 语法 + mapping 校验，不要求含 `providers`。

### 7.3 跨 Agent 迁移

- `MigratePage` 增加 omp 选项。
- `core/migrate.go` 的 `resolveMigratePath` 增加 `case AgentOMP: return OMPModelsWritePath()`；`.json`-only 目标直接返回“不可写”。
- 迁移仍走目标 backend 的 `Upsert`，字段映射自动生效。
- 迁移泄漏防护：
  - 目标 `Upsert` 只从目标旧文档保留未知字段；
  - 源 payload 只贡献 `base_url`、`api_key`、`kind`、cards；
  - 不从源复制 `compat`、`discovery`、`modelOverrides`、`properties`、`access` 等私有结构。
- 迁移警告：
  - 密钥会以明文写入 `models.yml`；
  - 源没有可用密钥（`!command` / 环境变量名 / OAuth）时，目标已有 `apiKey` 保留，不覆盖为空；
  - 自定义 / `off` 档位不会写入；
  - 结构化输出、联网搜索、中段 system、附件、温度、非 text/image 输入不会迁移；
  - provider 显示名不会迁移；
  - 迁移后不会自动设置默认模型（P1 需手动在“默认模型”中选择）。

#### omp 作为源（G17）

`MigratePage` 允许任意两个 agent，因此必须定义 omp → ZCode / DSH：

| 源状态 | 迁移行为 |
|---|---|
| `api` 是三种主流协议 | 正常迁移 `baseUrl` / `apiKey` / cards |
| `api` 是其余 10 种 | 该 provider 标记为“不可迁移”，预览里列出原因；不静默跳过 |
| `apiKey` 是 `!command` | 无法解析，**不迁移密钥**；provider 仍迁移，目标侧 Key 留空并提示 |
| `apiKey` 是环境变量名 | 同上，不解析；目标写空 Key，避免把变量名当密钥 |
| `apiKey` 是字面量 | 正常迁移 |
| `auth: oauth` 且无 Key | 迁移结构与模型，目标无 Key |
| `modelOverrides`-only / 无 `baseUrl` | 不可迁移，预览列出原因 |
| `discovery`-only | 不可迁移（APM 不展开发现结果） |
| 模型 `name` / `contextWindow` / `maxTokens` / `reasoning` / `input` / `supportsTools` | 迁移 |
| `thinking.mode` / `effortMap` / `cost` / `compat` / `headers` / `tokenizer` / `promptCache` / 模型级 `api` / 模型级 `baseUrl` | 不迁移；预览列出丢弃项 |
| 文件内 `compactionModel` / `contextPromotionTarget` | 不迁移 |

迁移预览必须显示“源私有字段丢弃清单”，且 `MigrateSide.format` 用 `models.yml` 而不是内部枚举名。

#### overwrite 语义

- `overwrite` 只覆盖 APM 管理的已知字段，目标 provider 的未知键、`compat`、`discovery`、`modelOverrides`、`cost`、`tokenizer` 仍保留。
- 不允许用 `overwrite` 把目标 provider 整体替换成源结构。
- 目标是只读 provider（不可映射协议 / `modelOverrides`-only / 无 `baseUrl`）时，`merge` 与 `overwrite` 都拒绝该条并计入 `skipped`，不降级为部分写入。

### 7.4 备份的跨 agent 隔离（G12）

`ExportBackup` / `ImportBackup` 目前是 agent 中立的：导出时只是把 `ProviderEdit` 序列化，导入时逐条走 `ImportProvider`。这带来两个泄漏面：

1. **导出**：omp 的 `_raw_provider` 可能带 `apiKey`、`headers.Authorization`、`compat`、`discovery`；`StripBackupKeys` 只按敏感键名递归剥离，不能保证 omp 的私有结构不被带出。
2. **导入**：从 omp 备份导入到 ZCode 时，`ImportProvider` 会重建编辑器，但 `_raw_provider` 若被当成基座，就会把 omp 的 `api` / `models[]` 结构写进 ZCode 的 JSON。

P1 要求：

- 导出时对 omp provider 走与迁移相同的“规范化投影”：只保留 `id` / `base_url` / `kind` / `api_key` / cards；`options_json` 只保留 `headers`（剥离敏感键）与 `authHeader`；`_raw_provider` 置空。
- 不可映射协议 / `modelOverrides`-only / 无 `baseUrl` 的只读 provider **不导出**，返回 `skipped` 列表与原因，不生成破损条目。
- `include_keys=false` 时，omp 的 `apiKey`、`headers` 里的凭据、模型级 headers 全部剥离。
- **空 Key 不覆盖**：导入 / 合并 / 迁移时，源 `api_key` 为空表示“没有可用密钥”，不能把目标已有 `apiKey` 删除；只有用户在编辑页显式清空才删除。否则“不含密钥的备份”会静默清掉用户现有的 Key。
- 导入时目标 backend 的 `Upsert` 仍以目标旧文档为基座（§6 第 5 条），源 `_raw_provider` 不参与写入。
- 只读 provider 导入冲突：`conflict=skip` 时跳过；`overwrite` 时若目标是只读 provider，拒绝该条并计入 `failed`，不降级为部分写入。
- `InspectBackup` 的 `agent_label` 必须来自 `AllAgents`，不能显示原始 `omp`。
- 备份信封只含 provider / model，不含 `config.yml` 的 `modelRoles`、`modelPresets`；UI 明确说明“默认模型不随备份迁移”。

---

## 8. 前端改动

| 文件 | 改动 |
|---|---|
| `frontend/src/stores/setting.ts` | `AgentID` 加 `'omp'`；`loadAgent()` 白名单；不要沿用“未知回落 zcode” |
| `frontend/src/layout/AppTopbar.vue` | `agentOptions` 加 `{ label: 'PI Agent (oh-my-pi)', value: 'omp' }`；建议改用 `ListAgents()` 动态生成，避免第三次硬编码 |
| `frontend/src/App.vue` | `normalizeAgentID`、`agentLabel`、启动回读两处白名单加 omp |
| `frontend/src/pages/MigratePage.vue` | `agentOptions` 加 omp；`labelOf` 回退；迁移警告；源/目标默认值仍可用 |
| `frontend/src/pages/ManagePage.vue` | `agentLabel` 加 omp（钥匙串备注）；只读横幅与只读时禁用保存 / 自动保存 / Ctrl+S / StickySaveBar `canSave=false`；默认模型卡片；ID 正则按 agent 放宽；`builtin:` 过滤按 agent 调整；kind 下拉对只读 provider 禁用并显示 `raw_api`；`options_json` 编辑器限制键域；明文 Key 警告 |
| `frontend/src/pages/ImportPage.vue` | omp 明文 Key 警告；provider_name 只读（显示 ID）；协议映射；`FetchModels` 传 kind；ZCode/legacy 卡已自动隐藏，但建议改成显式 `only zcode` |
| `frontend/src/pages/RestorePointPage.vue` | `formatLabel` 加 omp / omp-config；`.format-badge--omp` / `--omp-config` 样式；`loadCompat()` 限制 `agent === 'zcode'`；`KNOWN_OPS` 加 `set_default_model`；备份导入的 agent label |
| `frontend/src/pages/KeychainPage.vue` | 无需结构改动；备注依赖 ManagePage 的 `agentLabel` |
| `frontend/src/pages/OptionsPage.vue` | `customVariantsDesc` / `autofillDesc` 增加 omp 说明；omp 下“自填档位”“全强度”语义说明 |
| `frontend/src/components/ModelCard.vue` | omp 下隐藏自填档位、隐藏 off 胶囊、只显示六档；能力区只保留 input text/image 与 `tool_call`，隐藏 output 行与其余布尔位；默认档位下拉保留；ZCode 提示文案改为“DSH/omp” |
| `frontend/src/composables/useCapabilityFill.ts` | 增加“可用能力位”过滤（按 agent 或后端传入的集合），omp 下只允许 input text/image + tool_call |
| `frontend/src/composables/useLevelFill.ts` | omp 下档位集合限定六档；不要把 off / enabled 当强度档补入 |
| `frontend/src/composables/useVendorGroup.ts` | 依赖 `_vendor` / `_supported_levels`；后端必须填（§4.8） |
| `frontend/src/types/index.ts` | `RestorePointFormat` 加 `'omp'` / `'omp-config'`；`ProviderEdit` 加 `read_only` / `read_only_reason` / `api_key_kind` / `raw_api`；`ProviderSummary.read_only`；`ConfigLocation.needs_migration` |
| `frontend/src/i18n/index.ts` | omp 标签、只读、默认模型、明文 Key、格式、迁移警告、操作类型，三语齐全；同时把 ManagePage / ModelCard 现有硬编码中文纳入 `t()` |
| `frontend/src/api/index.ts` | `GetOMPModelRoles` / `SetOMPModelRole` wrapper；`FetchModels` 改为三参；`normalizeApiType` 增加 omp 映射 |
| `frontend/src/constants/variants.ts` | 注释补充 omp 六档；确认 `off` 只作为 UI 关闭语义，不写盘 |

UI 规则继续遵守 `UI-GUIDELINES.md`：

- 零 emoji；
- 零原生 `prompt` / `confirm` / `alert`；
- 不引入新的 UI 库；
- 颜色 / 圆角 / 时长 / 显现走 `global.css` + `theme.ts` token；
- 复用 `PageHeader`、`StickySaveBar`、`UiDialogHost`、`useToast` 等既有层。

---

## 9. 分阶段实施

### P0：地基修复（约 0.5 天）

- 修 `LoadConfigForRestore` / `RestoreBackup` 的 YAML 后缀判定。
- 补 DSH YAML 还原测试。
- 更新 `AllAgents()` 长度断言（P1 后为 3）。

验收：

- `go test ./...` 通过。
- DSH 的 `settings.yaml.bak_*` 可正常“恢复最近备份”。

### P1：最小完整兼容（约 3–4 周）

后端：

1. `internal/ompprovider`：Load / Sync / Validate / Bytes / Detect / Paths / Config。
2. `internal/core/agents.go`：AgentOMP、AllAgents、IsValidAgent、NormalizeAgentID、默认路径、格式名。
3. `internal/core/backend_omp.go` + `omp_store.go` + `omp_models.go`：
   - Read / Write / ReadWithFingerprint；
   - List / Get / Upsert / Delete / DeleteModel；
   - modelRoles / presets 引用保护；
   - 只读判定；
   - 默认模型 store/绑定辅助。
4. `internal/core/config.go`：
   - `DetectConfigLocationsForAgent` 增加 omp 默认/profile/legacy 候选；
   - `ConfigProviderSummaryForAgent` 增加 omp 分支；
   - `SetConfigPath` / `ChooseConfigFile` 拒绝 `config.yml`；
   - P0 的 YAML 恢复修复。
5. `internal/core/restorepoint.go`：FormatOMP / FormatOMPConfig、ConfigFormatAt、validateRestoreContent、`set_default_model` 操作、`omp-config` 还原后不改 target。
6. `internal/core/models.go`：ProviderEdit 只读字段、ProviderSummary.read_only、ConfigLocation.needs_migration。
7. `internal/app/app.go`：
   - `defaultPathForAgent` / `preferModernConfigPath` / `loadAgentState`；
   - `configFileAgent` 的 omp 识别与顺序（见 §9.1）；
   - `SaveProvider` 只读预检查；
   - `GetOMPModelRoles` / `SetOMPModelRole` 绑定（含 `selectors`）；
   - ZCode 专属绑定守卫（G8）：`GetZCodeFormat`、`GetZCodeCompat`、`RepairZCodeCompat`、`PreviewLegacyImport`、`ApplyLegacyImport`、`zcodeFormatTargetPath`、`ImportOpencode`。
8. `internal/core/probe.go` / `fetch.go`：
   - 新增带 headers 的探测/拉取入口（G5），由 `TestProviderConnection` / `RefreshProviderModels` 从 `ProviderEdit` 提取 `headers` 与 `authHeader`；
   - `headers` 优先于原生认证头；`authHeader: true` 时确保补 `Authorization: Bearer`；`authHeader: false` 不关闭 OpenAI/Anthropic SDK 的原生认证；
   - 跨主机重定向剥离所有敏感头（`Authorization`、`x-api-key`、自定义认证头），不只是 `Authorization`；
   - `apiKey` 是 `!command` 时直接返回“无法解析命令引用”，不执行；
   - 按 omp 解析规则处理环境变量名（`os.LookupEnv`，读不到就用字面量）。
9. `internal/core/migrate.go`：`resolveMigratePath`、格式名、omp 作为源的字段投影与不可迁移判定。
10. `internal/core/backup.go`：omp provider 导出时规范化投影、`StripBackupKeys` 覆盖 omp headers、`ImportBackup` 只读冲突。

### 9.1 `configFileAgent` 的识别顺序（G8 关联）

当前只认 DSH → ZCode。加入 omp 时不能简单在末尾追加，因为：

- DSH 的 `settings.yaml` 与 omp 的 `models.yml` 都是 YAML；
- ZCode legacy 接受“任何 JSON 对象”，会抢走 omp 的 `models.json`。

建议顺序：

1. 路径在 `OMPHome()`（含 `PI_CONFIG_DIR` 覆盖）或 `PI_CODING_AGENT_DIR` 下：
   - 文件名 `models.yml` / `models.yaml` / `models.json` → `omp` / `omp-models`;
   - 文件名 `config.yml` / `config.yaml` → 不是 provider 文件，返回空并提示“这是 omp 设置文件”；
2. 内容 sniff：
   - YAML 根节点是 mapping 且含 `providers` → `omp`；
   - YAML 根节点含 `llm-pi-ai` 或命中 DSH profile 结构 → `deepseek`；
   - JSON 顶层含 `provider` → `zcode` legacy；
   - JSON 顶层含 `schemaVersion` + `config` → `zcode` v2；
3. 都不命中 → 空。

`ConfigProviderSummaryForAgent` 对 omp 必须按 `providers` 读取；对 omp `config.yml` 返回 0 个 provider，而不是走 ZCode v2 分支。`agentID==""` 的总览模式也必须做同样的内容 sniff（当前实现会拿 YAML 去 `LoadConfig` 按 JSON 解析，omp 条目会显示 0 个 provider）。

`PreviewConfigMerge` / `ChooseMergeFile` / `MergeConfig` 的用户文案目前写“支持 ZCode 与 DeepSeek Harness”，改为三者并列出 omp。

> **G14 附注**：`ListAgents` 绑定已经存在但前端从未调用。P1 可以继续维护静态列表，但建议顺手接上 `ListAgents()`，否则 omp 之后每加一个 harness 都要改 5 处硬编码。

前端：

- 按第 8 节全部落地。
- 默认模型卡片（选项来自 `selectors`，支持清除与“当前值不在列表”）。
- 只读状态：横幅、保存禁用、自动保存 `preflight` 暂停、`canSave=false`、Ctrl+S 不落盘。
- 能力裁剪：ModelCard + 批量能力补全 + 批量档位，三处一致。
- 迁移警告与不可迁移清单。
- 三语文案 + 现有硬编码中文收口。
- RestorePointPage 的格式徽标、操作类型、ZCode 兼容卡守卫。
- 文档：README 增加 omp 支持范围与明文 Key 提示；CHANGELOG 记录 P0/P1；本文件的未决项与实现同步。

测试：

- `ompprovider` 文档往返、未知键保留、注释保留、数组按 id 同步、校验。
- 空文件 / 只有注释 / 根为 `null` / 根为 mapping 的创建行为。
- BOM、CRLF、`!command` 标量、锚点与别名的保留。
- provider 空 models 的允许键规则；删除最后一个模型后的行为；`providers: {}` 合法。
- `thinking` 缺失时的 fallback ladder；`efforts: []`；`off` / `enabled` / 自定义档过滤。
- `apiKey` 三种形态：字面量 / 环境变量名 / `!command`；`$VAR` 警告。
- 重复 model id、非法 provider id（`/`、空格）、`maxContextWindow < contextWindow`、`cost` 缺字段。
- core 路径解析（`PI_CONFIG_DIR`、`PI_CODING_AGENT_DIR`、`OMP_PROFILE`、命名 profile）。
- `.json`-only 时 Read 只读可见、Write/Upsert/Delete/DeleteModel 拒绝；`.yaml`-only 时写 `.yaml`。
- `config.yml` 不在 locations / 不可 SetConfigPath / 不可成为 target；恢复 `omp-config` 还原点后 target 仍是 models 文件。
- backend CRUD、指纹冲突、只读 provider 拒绝保存、只读 provider 可删除。
- `modelRoles` / `modelPresets` / 文件内 `compactionModel` / `contextPromotionTarget` 引用保护；Upsert / Delete / DeleteModel 三条路径。
- ZCode ↔ omp、DSH ↔ omp、omp → ZCode/DSH 迁移，断言源私有键不泄漏、不可迁移项被列出。
- `DetectConfigLocationsForAgent` profile 列表与 `needs_migration`。
- `ConfigFormatAt` / 还原点 / 默认模型写入 / `settings.json` 阻断。
- 删除默认模型被拒；切换默认后可删除；重命名被引用对象被拒。
- 探测与拉取：headers 透传、`authHeader: false`、跨主机剥离敏感头、`!command` 拒绝。
- 备份：omp 导出不含 `_raw_provider`；`include_keys=false` 剥离 apiKey/headers；导入只读冲突。
- ZCode 专属绑定在 omp 下返回 `WRONG_AGENT`，不读写 ZCode 文件。

验收：

- Topbar 可切到 `PI Agent (oh-my-pi)`，默认目标 `~/.omp/agent/models.yml`。
- 新建 provider + 2 个模型 → 文件生成；omp 沙箱可读取且无 validation failed。
- 编辑上下文 / 输出 / 推理档位 / 输入模态 → 重读一致；未知键与注释保留。
- 删除 provider / model → 正确；被默认模型引用时拒绝并给出中文原因。
- 设置 / 清除 `modelRoles.default` → `config.yml` 正确更新，注释与其他键保留。
- 默认模型下拉能列出全部 `selectors`；当前值不在列表时仍显示。
- 只读 provider 显示 `raw_api`，保存 / 自动保存 / Ctrl+S 均不写盘；后端直接调用 `SaveProvider` 也被拒绝。
- 只有 `models.json` 时顶部提示先运行 omp；APM 不创建 `.yml`。
- 恢复 `config.yml` 还原点后，当前 target 仍是 `models.yml`。
- 带有自定义 `headers` 的 provider 能探测成功；headers 里的认证头优先，`authHeader: true` 时补 Bearer。
- omp 备份导出不含 `_raw_provider`；关闭 include_keys 后无 apiKey/headers。
- omp 下 ZCode 诊断卡、旧版导入、OpenCode 导入、ZCode 格式卡全部隐藏且后端拒绝。
- 厂商分组、档位弱化、批量能力补全在 omp 下不出现 video/audio/pdf、输出模态等无效项。
- ZCode / DSH 配置完全不被触碰。
- 备份 / 还原点 / “恢复最近备份”可用，格式标签正确。
- `go test ./...`、`go vet ./...`、`npm run build`、`wails build` 全绿；浅色 / 深色人工检查。

### P2：完整体验（约 1–2 周）

- profile 下拉选择（默认 + 所有命名 profile）。
- `smol` / `slow` / `advisor` 等角色与 `modelPresets` 编辑。
- 全部 13 种 omp wire API 的协议选择器与 `thinking.mode` 选择器。
- `discovery`、`modelOverrides`、`cost`、provider/model headers 专用编辑。
- API Key 引用编辑：允许用户显式写成环境变量名 / `!command`；P1 只保留、不生成。
- 内置 provider ID 冲突提示。
- `models.yaml` / `models.json` 显式迁移。
- 只读诊断：未知 compat 警告、schema 问题、版本兼容提示。
- 迁移预览的字段丢弃清单与“迁移后设为默认”。

---

## 10. 测试与验证

### 10.1 仓库标准验证

```bat
cd frontend && npm ci && npm run build
cd ..
go test ./...
go vet ./...
go mod verify
node scripts/sync-version.mjs --check
node scripts/sync-model-catalog.mjs --check
node scripts/sync-ability-catalog.mjs --check
wails build
git diff --check
```

### 10.2 omp 真实沙箱验证（手工）

```bat
set PI_CODING_AGENT_DIR=%TEMP%\omp-apm-verify
mkdir %PI_CODING_AGENT_DIR%
:: 让 APM 把测试 provider 写到该目录的 models.yml（占位 key）
omp models --json 2>stderr.txt
```

通过标准：

- stdout 含目标 provider/model；
- stderr 没有 `models.yml validation failed`；
- 修改档位 / 删除模型后重读一致；
- 故意写坏 `apiKey` 能复现 `custom providers disabled`，确认 APM 的预校验能提前拦截。

注意：

- 只用临时目录与占位密钥；
- 不对真实 `~/.omp` 做写入测试；
- 不把真实 `agent.db` 或凭据复制到测试目录。

### 10.3 测试清单

| 类型 | 用例 |
|---|---|
| 单元 | 空文件 / 只有注释 / `null` 根 Load |
| 单元 | 根节点未知键与注释保留 |
| 单元 | provider 未知键保留 |
| 单元 | models 数组按 id 合并 / 覆盖 / 重复 id |
| 单元 | 删除 provider / model / 最后一个 model |
| 单元 | thinking 六档、off 过滤、空 efforts、缺 thinking 的 fallback |
| 单元 | 校验：缺 baseUrl / apiKey / api / 非法 maxContextWindow / cost 四字段 / 空 provider |
| 单元 | apiKey 三种形态、`$VAR` 警告、headers 引用 |
| 单元 | config.yml `modelRoles.default` 读写、注释保留、缺失文件创建、settings.json 阻断 |
| 单元 | 文件内 `compactionModel` / `contextPromotionTarget` 引用 |
| 集成 | AgentOMP 注册与 AllAgents 长度 |
| 集成 | 路径解析（默认 / PI_CONFIG_DIR / PI_CODING_AGENT_DIR / OMP_PROFILE / 命名 profile） |
| 集成 | `.json`-only / `.yaml`-only / `.yml`+`.yaml` 并存的读写选择 |
| 集成 | backend CRUD + 指纹冲突 + 只读拒绝 |
| 集成 | modelRoles / presets / 文件内引用保护（Upsert / Delete / DeleteModel） |
| 集成 | ZCode ↔ omp、DSH ↔ omp、omp → ZCode/DSH 迁移，源私有键不泄漏 |
| 集成 | FormatOMP / FormatOMPConfig 还原点、还原校验、target 不切换 |
| 集成 | config.yml 不在 locations / 不可 SetConfigPath |
| 集成 | ZCode 专属绑定在 omp 下返回 WRONG_AGENT |
| 集成 | 探测 / 拉取的 headers、authHeader、跨主机剥离、命令引用拒绝 |
| 集成 | 备份导出投影与导入冲突 |
| 手工 | 浅色 / 深色 UI、Topbar 切换、只读横幅、默认模型卡片、迁移页、还原点页、批量补全 |

---

## 11. 风险清单

| # | 风险 | 影响 | 对策 |
|---|---|---|---|
| 1 | omp schema 跨版本漂移 | 写坏配置，全部自定义 provider 被禁用 | 写前校验；保留未知键；18.16 实测，18.18 复测 |
| 2 | 只依赖退出码 | 误判写入成功 | App 内本地 Validate；手工验证解析 stderr |
| 3 | 迁移把源私有键写进 omp | 配置污染 / 静默无效 | 目标旧文档才是保留基座；源只贡献规范化字段 |
| 4 | YAML 重写丢注释 / 未知键 | 用户手写内容丢失 | `yaml.Node` 节点级同步；数组按 id 同步 |
| 5 | `modelRoles` 引用失效 | omp 默认模型指向不存在模型 | P1 删除 / 重命名 / 移除模型前拒绝；提示先切换默认 |
| 6 | 内置 provider ID 冲突 | 用户以为是新 provider，实际是覆盖 | P2 内置 ID 提示；P1 文档说明 |
| 7 | API Key 明文 / 引用歧义 | 密钥泄漏或 probe 失败 | 明文 + 警告；不执行 `!command`；P2 引用保真 |
| 8 | 能力位无法表达 | 用户以为已配置，实际无效 | UI 隐藏 / 标注；迁移预览列出丢弃项；写入端只写真实字段 |
| 9 | `.yml` / `.yaml` / `.json` 遮蔽 | 写进 omp 不读的文件 | 优先已存在文件；不写 `.json`；P2 显式迁移 |
| 10 | 与运行中的 omp 并发写 | 后写覆盖 / 文件占用 | 指纹 + 原子替换；失败提示重试；建议写前关闭 omp |
| 11 | 现有 YAML 备份还原 Bug | 还原最近备份失败 | P0 修 `LoadConfigForRestore` |
| 12 | 多处默认回落 ZCode | 误读写 ZCode 文件 | 所有 switch / 默认分支加 omp；测试“只动 omp 文件” |
| 13 | 项目级 config 覆盖全局 | 默认模型在特定项目中不生效 | UI 提示；P2 可检测 cwd 项目层 |
| 14 | 只有 `models.json` 时新建 `.yml` | 旧配置永被忽略 | P1 拒绝写入并提示先跑一次 omp；P2 显式迁移 |
| 15 | 空 models 的 provider 只剩 `api` | 整份 models.yml 失效，所有自定义 provider 被禁用 | 写前校验“允许键”；删除最后模型时同步删 provider 或拒绝 |
| 16 | `thinking` 缺失被当成空档位 | 读回丢档位 / 写回删 reasoning | 使用 omp 的 fallback ladder |
| 17 | `apiKey` 填 `$VAR` | omp 当字面量发送；探测误报 | 保存警告；探测按 omp 规则解析环境变量名 |
| 18 | 探测/拉取不带 headers / 忽略 authHeader | 网关永远探测失败，或与 omp 实际请求不一致 | 后端透传 headers；跨主机剥离敏感头 |
| 19 | `config.yml` 成为 target | 设置文件被当 provider 文件重写 | 排除出 locations 与 target；还原不切换 |
| 20 | ZCode 专属绑定无守卫 | omp 下读写 ZCode 文件、生成错误标签还原点 | `currentAgent == zcode` 守卫 |
| 21 | 批量补全写入 omp 无法表达的位 | 用户以为已配置，实际无效 | 后端裁剪 + 前端批量过滤 |
| 22 | 备份跨 agent 泄漏私有结构 | 目标 agent 被污染 | 导出规范化投影；导入以目标文档为基座 |
| 23 | i18n 只加新 key | omp 页面中英混排 | 同批收口 ManagePage / ModelCard 硬编码中文 |
| 24 | profile 支持口径不一致 | 实现范围反复 | P1 = 文件选择器 / 已探测位置；P2 = 下拉 |

---

## 12. 参考资料

### 本机源码

- `C:\AppData\BunData\global\node_modules\@oh-my-pi\pi-coding-agent\src\config\models-config-schema-bundle.ts`
- `...\src\config\models-config.ts`
- `...\src\config\model-registry.ts`
- `...\src\config\config-file.ts`
- `...\src\config\resolve-config-value.ts`
- `...\src\config\model-resolver.ts`
- `...\src\config\model-settings.ts`
- `...\src\config\model-presets.ts`
- `...\src\config\settings.ts`
- `...\src\cli\models-cli.ts`
- `C:\AppData\BunData\global\node_modules\@oh-my-pi\pi-utils\src\dirs.ts`
- `...\pi-utils\src\env.ts`
- `...\pi-ai\src\auth\cascade.ts`
- `...\pi-ai\src\auth\sqlite-credential-store.ts`
- `...\pi-catalog\src\compat\provider-ids.ts`
- `...\pi-catalog\src\compat\resolve.ts`
- `...\pi-catalog\src\models.ts`
- `...\pi-tui\src\overlays\model-selector.ts`

### 上游文档

- 仓库：<https://github.com/can1357/oh-my-pi>
- Providers：<https://github.com/can1357/oh-my-pi/blob/main/docs/providers.md>
- Models：<https://github.com/can1357/oh-my-pi/blob/main/docs/models.md>
- Settings：<https://github.com/can1357/oh-my-pi/blob/main/docs/settings.md>
- Config usage：<https://github.com/can1357/oh-my-pi/blob/main/docs/config-usage.md>
- Environment variables：<https://github.com/can1357/oh-my-pi/blob/main/docs/environment-variables.md>

### 本仓库相关实现

- `internal/core/backend.go`
- `internal/core/agents.go`
- `internal/core/config.go`
- `internal/core/restorepoint.go`
- `internal/core/migrate.go`
- `internal/core/provider.go`
- `internal/core/cards.go`
- `internal/core/model_features.go`
- `internal/core/probe.go`
- `internal/core/fetch.go`
- `internal/core/urlutil.go`
- `internal/core/backup.go`
- `internal/core/keychain.go`
- `internal/core/backend_deepseek_profile.go`
- `internal/core/deepseek_store.go`
- `internal/core/model_site_rules.go`
- `internal/core/capability.go`
- `internal/core/presets.go`
- `internal/dshprovider/patch.go`
- `internal/app/app.go`
- `internal/app/app_probe.go`
- `internal/app/app_backup.go`
- `internal/app/app_keychain_link.go`
- `MODEL-MANAGEMENT-PLAN.md` 10.1.3 / 10.2.3 / 10.15
- `UI-GUIDELINES.md`

---

## 13. 按推荐口径执行的默认项

以下不阻塞 P1；D11–D14 已确认，其余为安全默认，如要调整先改本节再实现：

1. P1 只管理 `modelRoles.default`，不管理 `smol` / `slow` / `advisor` 等其它角色。
2. 删除 / 重命名被默认模型引用的对象时拒绝，不自动改 `config.yml`。
3. P1 只支持三种主流协议，其余 10 种只读。
4. P1 **不生成** API Key 引用，但原样保留已有引用；探测时按 omp 规则解析环境变量名，不执行 `!command`。
5. P1 的 omp provider 显示名即 ID。
6. P1 的 `models.json`-only 场景只读可见、拒绝写入，不做自动迁移（D11）。
7. P1 的 `anthropic-messages` 不写自定义 `thinking`，档位由 omp 推导（D13）。
8. P1 的 `options_json` 只允许 `headers` / `authHeader`。
9. P1 的命名 profile 用已探测位置 / 文件选择器切换，P2 才做下拉。
10. 空 Key 不覆盖目标已有 Key；只有显式清空才删除（D15）。
11. 只读 provider 允许整体删除，禁止单模型删除与部分写入（D16）。
12. `config.yml` 永不做 provider 目标（D17）；glob 引用保守拒绝（D18）。

---

## 14. 全特性覆盖矩阵（第二轮）

下表按 APM 现有功能面逐项列出 omp 的适配结论。**“复用”** = 后端注册后自动可用；**“改造”** = 必须新增分支；**“只读/禁止”** = 入口保留但不可写；**“P2”** = P1 不做。

### 14.1 Agent 与路径

| 功能 | 现有实现 | omp 适配 | 阶段 |
|---|---|---|---|
| Topbar Agent 下拉 | `AppTopbar.vue:95-98` 静态列表 | 加 `omp`；建议改 `ListAgents()` | P1 |
| 启动回读 Agent | `App.vue:248/257` 白名单 | 加 `omp` | P1 |
| 页面状态重置 | `App.vue:18-22` `:key=agent` | 复用；切 agent 会丢页面内状态，可接受 | 复用 |
| `SetCurrentAgent` | `app.go:426` | 加 `defaultPathForAgent` / `preferModernConfigPath` | P1 |
| 默认目标路径 | `agents.go:61` | `case AgentOMP: OMPDefaultPath()`（活跃 profile 的读取路径） | P1 |
| 启动路径升级 | `app.go:116` 只遍历 ZCode/DSH | omp 没有 v1/v2 升级；只需在已存路径失效时回落到当前探测到的默认路径 | P1 |
| 配置位置探测 | `config.go:584` | 默认 / `PI_CODING_AGENT_DIR` / 命名 profile / `.yaml` / `.json` 标记；`agentID==""` 的总览分支也加 omp | P1 |
| 选择配置文件 | `app.go:469` | backend 试读；拒绝 `config.yml`；接受 profile `models.yml` | P1 |
| 设置配置路径 | `app.go:514` | 只允许探测到的 omp models 路径；拒绝 `config.yml` | P1 |
| 打开配置目录 | `app.go:502` | 复用 | 复用 |
| 目标状态 / 备份 | `app.go:448` | 复用；`ListBackups` 已按文件名前缀过滤，models/config 备份不混 | 复用 |
| 命名 profile 下拉 | 无 | P2 增加 | P2 |
| `ListAgents` | `app.go:408`，前端未用 | 建议接入，消除 5 处硬编码 | P2 |

### 14.2 Provider / Model CRUD

| 功能 | 现有实现 | omp 适配 | 阶段 |
|---|---|---|---|
| 列表 | `app.go:988` + `Store.List` | backend 注册后复用；`name=ID`、`kind` 映射、`read_only`、`model_count` | P1 |
| 详情 | `app.go:1017` + `Store.Get` | `read_only` / `raw_api` / `api_key_kind` / `options_json` 键域 | P1 |
| 保存 | `app.go:1045` + `Store.Upsert` | 只读预检、引用保护、写前校验、`_raw_provider` 不作为基座 | P1 |
| 删除 provider | `app.go:1188` | 引用保护；只读 provider 允许删除 | P1 |
| 删除 model | `app.go:1247` | 引用保护；有 backend 时不会走 `NormalizeConfigKinds`，仍建议显式排除 omp；前端当前不用该绑定，但必须正确 | P1 |
| 猜 ID / 猜协议 | `GuessProviderID` / `GuessKind` | 复用；`GuessKind` 只产出 3 种 UI kind，omp 下正好映射 | 复用 |
| 重命名 provider/model | 通过 `newID != providerID` | 视为删除+新增；旧 ID 被引用则拒绝 | P1 |
| 空模型保护 | `app.go:1081-1089` | 保留；但 discovery-only provider（无 models）需允许保存 baseUrl/apiKey | P1 |
| 新建 ID | `NewProviderID` UUID | 允许；另建议人类可读 ID 生成 | 复用 |
| ID 校验 | 前端 `^[a-zA-Z0-9_:\-]+$` | omp 禁止 `/`；`.` 允许。前端按 agent 放宽/收紧 | P1 |
| `builtin:` 过滤 | `ManagePage.vue:354/402` | omp 内置 provider 若以其他前缀暴露，需按 agent 调整；P1 不隐藏 omp 内置 | P1 |
| 自动保存 | `useAutoSave.ts` + 离开/退出 flush | 只读 provider 必须 `preflight` 暂停；引用保护失败时不反复弹错 | P1 |
| 钥匙串关联 | `app_keychain_link.go` | `agent` + `providerID` 组合键自动支持；`agentLabel` 修 omp | P1 |
| 明文 Key 警告 | 无 | omp 下强制提示，保存前确认 | P1 |
| 厂商分组 | `_vendor` | 后端填充 | P1 |
| 排序 / 折叠 / 搜索 | 本地 | 复用 | 复用 |

### 14.3 模型目录 / 拉取 / 站点规则

| 功能 | 现有实现 | omp 适配 | 阶段 |
|---|---|---|---|
| 拉取模型 | `FetchModels(baseURL, apiKey)` | 改为 `FetchModels(baseURL, apiKey, kind)`（两处调用点同步）；按 §4.5 映射 apiType；透传 headers；`apiKey` 引用按 omp 规则解析 | P1 |
| 重新获取 | `RefreshProviderModels` | 从 `ProviderEdit` 取 headers/authHeader/apiType，不依赖前端参数 | P1 |
| 单卡推断 | `BuildSingleCard` | 复用；omp store 负责 `_vendor` / `_supported_levels` | P1 |
| 自动匹配 | `ApplyModelPresets` | 复用；写入端裁剪 | P1 |
| 站点规则 | `model_site_rules.go` | 用 §4.5 的 omp→ZCode apiType 映射；否则限定协议规则失效 | P1 |
| `discovery` 拉取 | omp 自己的 discovery | P1 不用 APM 拉取替代；UI 注明“运行时还会发现更多模型” | P1 |
| `modelOverrides` | omp 内置模型覆盖 | 只读保留；P2 专用编辑 | 只读/P2 |

### 14.4 能力 / 档位 / 卡片 / 批量

| 功能 | 现有实现 | omp 适配 | 阶段 |
|---|---|---|---|
| 思考开关 | `ModelCard.vue:51-66` | 复用；关闭时删 `thinking` + `reasoning` | P1 |
| 档位胶囊 | `ModelCard.vue:70-108` | 隐藏 `off`；只显示六档；六档外的自定义档不显示 | P1 |
| 自填档位 | `ModelCard.vue:110-120` | omp 隐藏 | P1 |
| 默认档位 | `ModelCard.vue:121-130` | 映射 `thinking.defaultLevel` | P1 |
| 输入模态 | `ModelCard.vue:152-169` | 只保留 text/image；新建默认 text，已有值不强制改；空集合删除 `input` | P1 |
| 输出模态 | `ModelCard.vue:170-187` | 整行隐藏 | P1 |
| 布尔能力位 | `ModelCard.vue:188-211` | 只保留 `tool_call`；其余隐藏；取消 = 删键，不写 false | P1 |
| 推断来源 / 采纳 | `_capabilities_inferred` | 后端裁剪后才返回 | P1 |
| 批量思考 | `ManagePage.vue:152-174` | `variantOpts` 对 omp 去掉 off / enabled | P1 |
| 批量档位补全 | `useLevelFill.ts` | 六档；不补 off | P1 |
| 批量能力补全 | `useCapabilityFill.ts` | 按可用位过滤 | P1 |
| 全强度设置 | `reasoningAllIntensities` | omp 下只补六档 | P1 |
| 上下文 / 输出 | `ModelCard.vue:132-139` | `contextWindow` / `maxTokens` | 复用 |
| `maxContextWindow` | 无卡片字段 | 保留旧值并按规则抬高 | P1 |
| `cost` | 无卡片字段 | 保留；新模型不生成 | P1 |
| 模型 headers | 卡片有字段、UI 未编辑 | 保留 | 复用 |

### 14.5 连接探测

| 功能 | 现有实现 | omp 适配 | 阶段 |
|---|---|---|---|
| 原始探测 | `TestConnectionRaw(baseURL, apiKey, kind)` | UI kind 映射；无法映射时明确“不支持探测” | P1 |
| Provider 探测 | `TestProviderConnection` | 读取 `headers` / `authHeader` / `raw_api` | P1 |
| `/models` 阶段 | `FetchModelsRaw` | 透传 headers；值为 `!command` 的 header 不发并提示；跨主机剥离敏感头 | P1 |
| chat 阶段 | `probeChat` | 按 api 映射 endpoint 与原生认证；`headers` 优先，已有 `Authorization` / `x-api-key` 时不覆盖；`authHeader: true` 时确保补 Bearer | P1 |
| 引用型 Key | 无 | `!command` 拒绝；环境变量名按 omp 规则解析（只在 APM 进程环境里查）；读不到则按字面量发送，并在结果里提示 | P1 |
| `auth: oauth` 无 Key | 无 | 跳过探测并提示“凭据由 omp OAuth 管理”，不发请求 | P1 |
| `auth: none` | 无 | 不带认证探测 | P1 |
| 本地 provider | SSRF 默认拒绝 | 提示开启 HTTP / 私网放行（omp 的 ollama/lm-studio 常见） | P1 文案 |
| 探测诊断 | 只看 HTTP 状态 | 可附加“schema 可能失败”的提示（P2） | P2 |

### 14.6 导入 / 迁移 / 诊断

| 功能 | 现有实现 | omp 适配 | 阶段 |
|---|---|---|---|
| 表单导入 | `ImportProvider` | backend 自动；只读冲突拒绝 | P1 |
| ZCode 旧版导入 | `PreviewLegacyImport` / `ApplyLegacyImport` | omp 下隐藏 + 后端守卫 | P1 |
| OpenCode 导入 | `PreviewOpencodeImport` / `ImportOpencode` | 前端未用；后端加 omp 拒绝，避免落通用 JSON 写 YAML | P1 |
| 配置合并 | `PreviewConfigMerge` / `MergeConfig` | `configFileAgent` 识别 omp；omp 可作源/目标；`config.yml` 不是 provider 文件 | P1 |
| 跨 Agent 迁移 | `MigratePreview` / `MigrateExecute` | 双向；不可迁移清单；overwrite 只覆盖已知字段 | P1 |
| ZCode 格式卡 | `GetZCodeFormat` | omp 下隐藏 + 守卫 | P1 |
| ZCode 目标路径 | `zcodeFormatTargetPath` | 非 ZCode 时目前强制返回 ZCode 默认路径；omp 下必须短路，不能再读 ZCode 文件 | P1 |
| ZCode 兼容诊断 | `GetZCodeCompat` / `RepairZCodeCompat` | omp 下隐藏 + 守卫；禁止写 ZCode 文件 | P1 |
| 通用诊断 | 无 | 可加 `ValidateOMPConfig` 只读报告 | P2 |
| 备份导入 | `ImportBackup` | backend 自动；只读冲突 | P1 |

### 14.7 备份 / 还原点 / 钥匙串

| 功能 | 现有实现 | omp 适配 | 阶段 |
|---|---|---|---|
| 最近备份 | `GetBackupInfo` / `RestoreLastBackup` | 复用；只作用于 `models.yml` | 复用 |
| YAML 还原 Bug | `LoadConfigForRestore` | P0 修后缀判定 | P0 |
| 还原点列表 | `GetRestorePoints` | `FormatOMP` / `FormatOMPConfig`；agent label 自动 | P1 |
| 恢复还原点 | `RestoreRestorePoint` | `config.yml` 只回写内容，不切换 target | P1 |
| 删除 / 清理 | `DeleteRestorePoint` / `PruneRestorePoints` | 复用 | 复用 |
| 导出备份 | `ExportBackup` | omp 规范化投影；剥离 `_raw_provider`；Key 模式复用 | P1 |
| 检查备份 | `InspectBackup` | agent label 从 `AllAgents` 取 | 复用 |
| 导入备份 | `ImportBackup` | 只读冲突；目标文档为基座 | P1 |
| 全局钥匙串 | `keychain.go` | 复用（DPAPI、500 条上限） | 复用 |
| 关联 | `keychain_links` | 键已含 agent；只修 label；引用型 Key 关联钥匙串会变成字面量，保存前确认 | P1 |
| 明文风险 | 无提示 | 备份 / 还原点 / `.bak_` 都含明文 Key，UI 提示 | P1 文案 |

### 14.8 前端壳 / i18n / 选项

| 功能 | 现有实现 | omp 适配 | 阶段 |
|---|---|---|---|
| Agent 标签 | `App.vue:119`、`ManagePage.vue:920` | 加 `PI Agent (oh-my-pi)`；钥匙串备注修 | P1 |
| 格式标签 | `RestorePointPage.vue:315` | 加 `omp` / `omp-config` + CSS | P1 |
| 操作类型 | `KNOWN_OPS` | 加 `set_default_model` | P1 |
| 只读 UI | 无 | 横幅 + 保存/自动保存/Ctrl+S 禁用 | P1 |
| 默认模型卡 | 无 | 新卡片 + 三语文案 | P1 |
| 迁移警告 | `migrate.*` | omp 专用丢弃清单 | P1 |
| 能力文案 | `cap.*` | omp 只支持 text/image + tool_call 的说明 | P1 |
| 档位文案 | `reasoning.*` | 六档、无 off、anthropic 推导提示 | P1 |
| 明文 Key 文案 | 无 | `manage.ompPlaintextKey*` 等 | P1 |
| 硬编码中文 | ManagePage / ModelCard 多处 | 同批收口，三语补齐 | P1 |
| 主题 / 语言 / 强调色 | 全局 | 复用 | 复用 |
| `GetAppInfo` | `app.go:231` | `agents` 自动含 omp；`variants` / `ui_variants_by_kind` 是 ZCode 全集，omp 仍用同一 UI kind，前端按 agent 裁剪即可 | P1 前端 |
| HTTP / 私网 / 自动填充策略 | 全局 | 复用；omp 本地 provider 需提示 | 复用 |
| 自填档位 / 全强度开关 | `OptionsPage` | omp 下说明不适用 | P1 文案 |
| 更新器 | `internal/update` | 无关 | 复用 |

### 14.9 明确不需要为 omp 改的部分

- 更新器、版本同步、发布资产。
- 主题 / 语言 / 强调色 / 窗口壳。
- 表情/对话框/UI 库约束（继续遵守 `UI-GUIDELINES.md`）。
- 钥匙串加密格式（DPAPI）。
- SSRF / 重定向 / `RedactSensitive` 基础实现（只在 headers 透传处扩展剥离范围）。
- `models.db`、sessions、logs、`agent.db`、OAuth token：不读、不写、不导出。
- `!command`：永不执行。
