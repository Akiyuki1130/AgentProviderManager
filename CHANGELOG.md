# Changelog

## 2.1.1-pre — 2026-10-14

### 新增

- **版本号单一真源。** 仓库根目录的 `VERSION` 文件现在驱动版本出现的每一处，由 `scripts/sync-version.mjs` 分发（带 `--check` 模式）。Go 侧读取生成的 `internal/core/version_gen.go`（`core.AppVersion`，由 `main.go` 与 `GetAppInfo` 使用），侧栏不再硬编码：`AppSidebar.vue` 启动时从 `GetAppInfo` 读取运行版本。`go test ./...` 新增 `TestVersionSourcesInSync`，一旦与 `VERSION` 漂移即失败并打印修法，因此发版只需改 `VERSION` 再跑一次脚本（覆盖 `wails.json`、`frontend/package.json` 及其字节一致的 `package.json.md5`（Wails 用它判断是否重装依赖）、`frontend/package-lock.json` 与 `README.md`）。
- **两段式连接探测（`测试连接`）。** `core.TestConnection` 先复用加固过的取模型链路（SSRF 门、重定向姿态、错误码）访问 `/v1/models`，再用首个发现的模型向该协议的真实端点（`/chat/completions` / `/messages`，后者带 `x-api-key` + `anthropic-version` / `/responses`）发送 1-token 对话请求，因此鉴权是在真正承载流量的端点上得到验证的。响应按段报告 ok/延迟/数量，沿用既有的 `ModelFetchError` 错误码分类（DNS `CONN`、`TIMEOUT`、`HTTP_401`/`HTTP_404`、`BAD_URL`），所有消息都经 `RedactSensitive` 处理。以 `TestConnectionRaw`（表单值）与 `TestProviderConnection`（对已存 provider 走与 `RefreshProviderModels` 相同的后端凭据解析，DSH 的 `apiKeyEnv` 密钥可透明解析）暴露；已接入提供商编辑区与导入页，内联显示 ok/warn/err 结果行。
- **钥匙串 ↔ 提供商联动。** API Key 字段新增 `从钥匙串`（对受 DPAPI 保护的条目做可搜索的拾取弹窗并填入字段）与 `存入钥匙串`（把当前密钥存为具名条目）。所选关联记录在应用侧 `settings.json`（`keychain_links`，`agent|providerID` → 条目 id——ZCode 严格的 v2 schema 禁止在 `provider_config.json` 内做间接引用，因此 harness 配置仍保留解析后的真实值），在字段下方以同步 chip 呈现（覆盖 / 更新钥匙串 / 解除），钥匙串行上显示 `N 提供商` 徽标。绑定：`GetKeychainLinks`、`SetKeychainLink`、`RemoveKeychainLink`。
- **可移植备份导出 / 导入（`apm-backup v1`）。** 还原点页现在把当前 Agent 的提供商（全部或指定）导出为 JSON 信封，并经既有 `ImportProvider` 路径导回，因此每个导入的提供商都会自动获得自己的备份 + 还原点。是否含密钥与加密方式是两个正交选项：不含密钥（默认，可安全分享——`StripBackupKeys` 清除 `APIKey`、`options_json` 内的 `apiKey` 以及原始行）或含密钥；载荷可封为明文（plain——含密钥导出需要一次显式的“仍然导出”确认弹窗）、默认加密（sealed：以程序内置种子的 SHA-256 为密钥的 AES-256-GCM，任意设备上的本程序均可解密，能防止被直接当作明文读取，但不防有人提取该种子）或密码加密（标准库 `crypto/pbkdf2` SHA-256，60 万次迭代 + AES-256-GCM；元数据在解锁前保持锁定——`InspectBackup` 返回 `needs_password`，错误密码会在 GCM tag 上失败）。用不含密钥的备份覆盖已有提供商时会保留其本地 API Key（与 `MergeConfig` 的保护一致），自定义 headers 等非敏感 `options_json` 能经导入往返。文件以 0600 写入；密码绝不落盘。
- 管理页的提供商侧栏过滤与排序：一个匹配名称/id 的搜索框加一个排序下拉（默认顺序 / 名称 / 模型数），表头计数切换为 `filtered / total`。
- **模型能力表与自动补全（阶段 B）。** 新增 `scripts/sync-model-catalog.mjs`：从 ZCode 自带资源 `zcode-builtin.json` 抽取 `modelConfigRules.modelRules` 的 84 条家族规则，生成预编译 Go 表 `internal/core/model_catalog_gen.go`（`(?i)^(?:…)$` 全串锚定 + 大小写不敏感，与 ZCode 内部分配器一致；`--check` 幂等校验，可作为提交前手工闸门）。`internal/core/capability.go` 的 `LookupModelCapability` 按「APM 自维护覆盖层（`internal/core/model_overrides.go`）> 内建规则逐字段覆盖 > `presets.go` 兜底」取值，并同时给出厂商归属（19 个家族正则）。有意跳过 `.*` 纯兜底规则，未知模型保持「留空」契约。此前因前缀匹配断裂而匹配不到的 `deepseek-v4.1-flash` 等模型现在能自动补全上下文（1000000）、输出（384000）与档位（off/low/high/max）；`GuessReasoning` 改由能力表驱动（32 个代表模型比对：零 `true→false` 回归、8 个 `false→true` 修正）。
- **思考档位自适应 + 「允许自填推理档位」高级开关（阶段 C）。** 模型卡片按能力表弱化显示该模型不支持的档位（虚线胶囊，仍可手动点选），开启思考时以支持集合作为种子。选项页新增高级开关（写入 `settings.json`，默认关闭）：打开后档位名跳过白名单（仍去重、归一别名、限长 64 字符并拒绝控制字符），前后端同一上限；DSH 读写两端对称放行，自填档位不再「写进去、重读即消失」。
- **厂商分组视图（阶段 D，仅 APM 内部）。** 管理页与导入页的模型列表都支持「按厂商分组」开关（`localStorage` 记忆，两页共用同一偏好）与厂商多选筛选（选项按当前模型集动态生成、带计数、unknown 排最后，列表整体换掉后自动收敛掉已消失的筛选项）。逻辑收敛为唯一实现 `frontend/src/composables/useVendorGroup.ts`、外观收敛到 `global.css` 的 `.vendor-group*`，两页不再各写一份；手动添加的模型也会向能力表补一次厂商归属与支持档位（走既有 `BuildSingleCard`，只取内部 `_` 前缀 meta，不填上下文/输出）。分组只在渲染层，批量操作作用域不变（仍为当前筛选出的可见模型），分类结果绝不写入配置文件。
- **模型能力表扩容为五层信源（2026-10-15）。** 能力推断从「ZCode 内建 84 条正则 + 手维护 presets」扩为 **provider 自报 > APM 覆盖层 > 站点规则 > ZCode `modelRules` > presets > 社区离线目录**，逐字段覆盖、越靠前越权威。新增 `scripts/sync-ability-catalog.mjs` 生成 `internal/core/capability_catalog_gen.go`（**2305 条精确 id**：DSH 内嵌 pi-ai 目录 994 条 + models.dev 快照 1311 条；pi-ai 侧逐文件 SHA-256 校验 `.manifest.json`；`--check` 完全离线且跨天幂等，`--update-snapshot` 才联网刷新裁剪快照 `internal/core/data/models-dev-snapshot.json`，`--verify` 与 OpenRouter `/api/v1/models` 交叉校验但只报告、不并入）。`scripts/sync-model-catalog.mjs` 追加产出 ZCode `providerSiteRules` 54 条站点规则（`internal/core/model_site_rules_gen.go`），匹配语义以应用内实现实测为准（模型名全串锚定 + 大小写不敏感；`baseUrl`/`apiType` 全串锚定 + 大小写敏感；`baseUrl` 先摘 search/hash 与尾部 `/`）。新增查询入口 `LookupModelCapabilityForSite` / `BuildModelCardsForSite` / `CfgToCardForSite` / `ApplyModelPresetsForSite`（旧函数保留并转调，行为不变）。
- **拉取模型时读取 provider 自报的能力字段。** 此前 `/v1/models` 的响应对象只被读 5+5 个 token 键、思考档位整体丢弃；现在新增 `metaLevels`（识别 `thinking_level` / `thinkingLevel` / `reasoning_effort(s)` / `supported_efforts` / `reasoning_options[type=effort].values` / `supported_parameters`）与 `metaReasoning`，**provider 自报的档位优先于能力表**写入 `_supported_levels`（仍是内部字段、不写盘），上下文键表补 `max_input_tokens` / `context_size`，输出键表补 `output_token_limit` / `max_new_tokens`。实测效果（用户的两个网关提供商 `CMD GOAT 猫猫` / `CMD GOAT 秋雪`，同一 `api.commandcode.ai`，90 个模型）：**档位覆盖 43/90 → 79/90，上下文/输出 51/90 → 90/90**；该网关只自报 `context_length`（无输出上限、无档位字段），增益全部来自新信源与覆盖层。

- **模型能力维度补齐（阶段 E，2026-10-15）。** 能力层从「上下文 / 输出 / 档位」扩成 **harness 中立**的超集：输入模态 5 位（文 / 图 / 视 / 音 / PDF）、输出模态、工具调用、结构化输出、原生联网搜索、对话中途 system、附件、温度支持。新增 `internal/core/model_features.go`（`ModelFeatures`，**全部三态：`nil` = 未知，不是「不支持」**）与并列表 `internal/core/capability_features_gen.go`（**2320 条精确 id**，编码 `<输入模态>|<输出模态>|<flags>`，flags 固定序 `tjwmaT`，解析 fail-closed；与既有 `ctx|out|levels` 表彼此独立，旧解析契约不动）。求值链各层（override / ZCode 正则 / 站点规则 / presets / 离线目录）与新增能力位走**同一套逐字段合并**语义，`.*` 兜底规则照旧跳过。三个 harness 各自映射：ZCode v2 → `inputFormat.supports{Text,Image,Video,Audio,Pdf}` / `outputFormat.supportsText` / `supportsToolCall` / `supportsJsonSchemaOutput` / `supportsNativeWebSearch` / `supportsMidConversationSystem`；DSH → `input`（收敛到 `{text,image}`，交集为空则整键省略）+ `compat.supportsStrictMode` / `compat.supportsTemperature`；opencode → `modalities{input,output}` / `attachment` / `tool_call` / `temperature`。**落盘策略是「只写推断为真的位」**：未知与「假」一律省略键（ZCode 的 overlay 里省略 = 不覆盖，`null` 才是显式清空），因此能力不全不会弄坏任何配置，也绝不写 `false` / `null` / `enabled` / `requiresMfjsToolSchema` / `optionSpecs.*.map`。
- **能力数据源加宽（阶段 E）。** `scripts/sync-model-catalog.mjs` 从 ZCode 内建数据抽出能力位列（`modelRules` 84 条中 82 条带位、`providerSiteRules` 54 条中 45 条带位）；`scripts/sync-ability-catalog.mjs` 新增 pi-ai 的 `input[]` 与 `compat` 显式白名单、models.dev 的 `modalities{input,output}` / `tool_call` / `structured_output` / `attachment` / `temperature`（裁剪快照重裁 367KB → 803KB，155 provider / 3799 条目，`--check` 仍完全离线幂等）。同一个 id 被多个来源覆盖时**取并集**——pi-ai 的 `MODALITIES` 词表只有 `text,image`，给不出 pdf/video/audio 并不等于否定它们。
- **拉取模型与手工添加时也读能力位。** `BuildModelCards` / `CfgToCardForSite` 从 provider 自报字段（`modalities.*`、`input_modalities`、`architecture.input_modalities`、`capabilities.*`、`supports_*` 等）与能力表共同填卡片的 `capabilities`（provider 自报优先、API 事实始终写入，能力表部分仅在「自动补全」开启时填），并把原始推断值另存为内部字段 `_capabilities_inferred` 供 UI 展示来源与一键补全；两者都绝不落盘。

- **UI 中不再有任何 emoji（i18n 字典里也没有）。** 所有图形化字形（按钮、弹窗标题、徽标、`✔✕⚠` 状态字符、混进翻译字符串里的 emoji 前缀）都被替换为 `@vicons/ionicons5` 图标，经 naive-ui `<n-icon>` 渲染在按钮的 `#icon` 插槽里，因此字形跟随 accent 色、共享基线并随 size 属性缩放。对 `frontend/src` 做全量 emoji 码点扫描现在返回 0。
- **八处原生 OS 弹窗全部移除。** `prompt()` / `confirm()` 被主题化弹窗宿主（`components/ui/UiDialogHost.vue` + `stores/uiDialog.ts`，暴露 `uiConfirm` / `uiPrompt`）取代，在 `App.vue` 里挂载一次。删除类确认把正按钮样式化为 `error`（`danger: true`）；文本输入框内联校验并回车提交。UI 无法再触发 OS 绘制的 prompt、confirm 或 alert。
- **可复用组件 + composable 层。** 新增 `ui/PageHeader.vue`（标题 / 副标题 / 图标 / actions，被五个卡片式页面采用——导入 / 钥匙串 / 迁移 / 选项 / 还原点）、`ui/StickySaveBar.vue`（管理页的脏态保存条），以及 composable `useToast`（hover 暂停倒计时、语义图标、关闭按钮、最多 5 条、`aria-live="polite"`）、`useStagger`（按稳定 id 缓存错峰延迟，列表换序不会重播动画）、`useHighlight`（大小写不敏感的 `<mark>` 搜索高亮，渲染为转义分段——不用 `v-html`）。
- **设计 token 层。** 动效 token（`--dur-standard` / `-lg` / `--dur-settle` / `--dur-toast` / `--dur-page-leave` / `--dur-reveal` / `--ease-standard` / `--delay-step`）替换了全站硬编码时长；语义色集（`--ok` / `--warn` / `--err` 及 `-soft` 色调变体，含明暗两套色板）替换了五处临时硬编码的绿/红/橙。圆角改为比例缩放（控件高度 ×≈0.28：tiny 6 / small 8 / medium 10 / large 12 / 容器 12），不再用各页松散取值。新增 FLIP 列表与网格过渡（`.list-*`、`.grid-*`）以及共享的 `fade-in` / `.enter-stagger` 处理列表重排与页面入场。
- **`global.css` 里的规范控件样式。** `.field`、`.switch`、`.check`、`.badge`（三种语义变体）、`.pill` 现在有单一全局定义，页面复用而非重复声明，修掉了深色主题缺陷（不可见的开关轨道、写死的 `#888` 勾选边框），改用主题派生的 `color-mix` 颜色，补上 `:active` 状态，并让每个自绘控件都带 `role` / `aria-checked` / `tabindex` 及 Enter/Space 处理。
- **基础打磨。** 全局 `:focus-visible` 聚焦环（对自带聚焦态的 naive-ui 控件抑制）；`tabular-nums` / `mono` 工具类，以及为路径、ID、日志提供豁免全局禁选的 `.selectable`；隐藏滚动条；并用 `index.html` 中一段同步的 `<head>` 脚本在 CSS 包加载前涂好正确底色（消除冷启动白闪）。
- **深色主题与布局修复。** `theme.ts` 新增 `accentTextOnAccent`（按 WCAG 亮度在 accent 上取黑/白）、深色下按压态改为变亮而非变暗、以及按尺寸区分的按钮圆角；顶栏重新可整体拖拽（拖拽作用于整个头部，`no-drag` 收窄到真正的控件，因此路径胶囊也能随之拖动）；目标路径胶囊现在渐进折叠中间段（`盘符\…\file`）以便长路径可读，完整路径放在 tooltip 里；窗口控制与工具栏按钮在容器宽度低于 1080px 时收起文字标签（纯图标，经 `ResizeObserver` + 容器查询）；选项页用 accent 色板拾取器（八个预设 + 自定义）替换了原始颜色输入。
- **行为修正（仅前端）。** 选项页的开关现在返回后端结果并在失败时用错误 toast 回滚，而不再先弹成功 toast 再吞掉错误；钥匙串页与导入页显示区分开的 loading 与错误态，不再把失败伪装成“暂无条目”；批量上下文/输出输入框改文案为“输入后回车应用”并在回车时校验；钥匙串的显示标签与实际的全局显示状态一致。
- **其余页面的 i18n 补齐。** `KeychainPage` 与 `MigratePage` 完全接入 `t()`，导入页的硬编码字符串移入字典，`main.ts` 里的全局错误 toast 也已本地化；三套字典（zh / en / ja）保持同步，字符串本身的 emoji 前缀已移除（图标留在模板层）。
- **按 Agent 门控的 ZCode 区块。** ZCode 配置格式卡与旧版导入卡是 ZCode 独有的概念，在 DeepSeek Harness 下不再渲染（也不探测）；后续区块相应前移其错峰入场序号，避免出现动画空档。
- **模型卡档位交互。** 不支持的档位改为虚线弱化胶囊（仍可点开），支持集合来自能力表；档位集合不含 off 词的模型（如 `glm-5.3`、`kimi-k2.7-code`）思考开关禁用并给出原因；ZCode 侧的「默认档位」控件加「仅对 DSH 生效」提示；批量档位操作提示文案与实际作用域（当前筛选出的可见模型）一致。

- **模型卡的「能力」区与批量能力行。** 模型卡展开区新增「能力」区：输入模态、输出模态两组胶囊多选，加六个布尔位开关（工具调用 / 结构化输出 / 原生联网搜索 / 对话中途 system / 附件 / 支持温度参数），每块都写明「未勾选 = 未知，交给目标客户端自己的默认值，只写入确认支持的能力」；能力表推断出来的位以弱化样式标「推断支持」并可逐位「采纳推断」（绝不自动写入，渲染期零写入）。管理页与导入页的批量栏各加一行「能力」：按当前模型集动态生成、带计数的多选加「按能力表补全」按钮（**只补空缺**，已有选择一律不覆盖，空选 = 补全部可补项），完成后按实际补到的位数提示，逻辑收敛为唯一实现 `composables/useCapabilityFill.ts`。

- **档位（思考等级）批量补全与单卡「采纳推断档位」（阶段 F，2026-10-15）。** 模型卡的推理档位区新增「采纳推断档位」按钮（推断里有当前集合还没有的档位时露出），管理页与导入页的批量栏各加一行「档位」：按当前模型集动态生成、带计数的档位多选，加一个「按能力表补全档位」按钮。语义是**只增不减地并入**（不是整组替换）：把推断集合（`_supported_levels`）里当前集合还没有的档位加进去，**从不删除任何已有档位**——集合为空的新卡片并入结果就等于推断集合（「整组采纳」是它的特例），而存量卡片（APM 早先写下的盲目三档 `[off,high,max]`）也能被真正补上缺失的档位。替换式语义被否决：推断集合里有 18/79 只有 1–2 档（多来自社区目录、数据可能稀疏），替换会窄化用户自己挑过的集合，而写入比事实更多的档位只是多给几个可选项、比事实更少才是声明「不支持」。跳过条件：思考未开（那是用户的显式决定，且落盘会把集合收敛成单个 off 词、补进去的值会被静默丢弃）、推断里没有非 off 档（只有 off 与「思考已开」自相矛盾）、推断档位当前都已存在。逻辑收敛为唯一实现 `composables/useLevelFill.ts`（单卡与批量共用 `fillableLevels` / `applyLevels`），选项生成 / 计数 / 失效选择收敛 / 汇报与 `useCapabilityFill` 同构；补全只改卡片的 `variants` / `default_variant`，由两页既有载荷提交，不新增接口、不直接写盘。实测（用户两个网关 90 个 id）：有档位推断 79 个且全部可补——新卡片场景并入 298 个档位（其中 43 张并入后不含 off 档，前端按既有 S1 语义禁用该卡的思考开关，其中 19 张源于 ZCode 内建规则本身就没有关闭档）；**存量盲目三档场景并入 166 个档位、79 张卡的思考开关全部保持可用**。详见 `MODEL-MANAGEMENT-PLAN.md` 10.10。

- **档位 UI 重做 + 新增「覆盖」方向（阶段 F 修订，2026-10-16，人工验证反馈）。** 真实窗口目测后改四处：① **删掉批量栏的「选择要补全的档位」多选**——它的标签承诺「只补这些档位」，实际写入永远是整组集合（`当前集合 ∪ 缺失档位`），选择只改变「处理哪些卡片」，无法表达任何用户意图；补全现在是**无参数的一次性动作**（点一次、全部可补项全补）。② **档位补全/覆盖并入「推理档位」那一行**（紧跟胶囊组、在「默认档位」之前），批量栏不再有独立的「档位」行——此前第 2 行与最后一行都在写 `card.variants`，两个名字、上下两处，而补全按钮离它作用的胶囊最远。③ **模型卡的「推理档位 / 自填档位 / 默认档位」三个等重高亮块合并为一个块**（三者编辑的是同一组字段，拆开只会主次不分），块内每行一个子项：推理档位（胶囊 + 采纳 + 覆盖）→ 自填档位 → 默认档位。④ **新增「覆盖」方向**：批量栏是补全按钮后的复选框「覆盖已有档位」（默认不勾选、不持久化），单卡是「覆盖为推断档位」（仅当**确有档位会被删除**时露出——没有多余档位时它与「采纳推断档位」结果相同，同时给两个按钮只会让人犹豫）。覆盖把 `variants` 直接置为推断集合，是唯一能清掉「APM 早期盲目写入的档位」（实测 43 个模型带并不存在的 off / max）的动作，因此必须过 `uiConfirm`（danger）并聚合列出将删除的档位名，外加一句「其中 N 个模型的思考开关会变灰」（S1 语义：集合不含关闭档就无法关闭思考）。实现上 `overwriteLevels` 与 `fillableLevels` **同源不合并**：共用 `inferredLevels` 与同一套门控（思考未开、推断里没有非 off 档一律跳过——否则覆盖会把 `variants` 清空或置成 `["off"]`，等于**静默把用户的思考关掉**），但判据是「与推断集合**不相等**」而不是「有缺失档位」，复用后者会让「无缺失但有多余档位」这类最需要覆盖的卡片一张都不处理。顺带两处体验修正：一个都处理不了时说出原因（「N 个模型尚未开启思考，本操作只在思考已开启时生效」），「采纳推断档位」的悬浮提示改为列出**本次会并入的档位**。**能力位的多选保留**——能力是十几个彼此独立的位，"只补输入模态、先不确认布尔位"可以精确表达；差异来自**字段形状**（多个离散位 vs 单一整体集合字段），不是实现偷懒。i18n 删 2 键（`level.batchBitsPlaceholder` / `batchArrayNote`）、改 2 键、增 19 键（三语齐全、零 emoji）。无后端改动、不触碰任何写入路径。详见 `MODEL-MANAGEMENT-PLAN.md` 10.11。

### 修复

- **本轮审查修复（2026-10-16）。** 针对安全审计与能力层复核后暴露的若干遗留与回归，补齐如下：
  - **凭据同源闸门补齐到「测试连接」路径。** 此前只有取模型与备份导入两处做了同源校验，测试连接路径漏掉了：在已保存提供商上清空 Key 输入框、同时改写 Base URL 时，程序会沿用本地真实 Key 把它发往新主机。现在 `internal/app/app_probe.go` 走与 `RefreshProviderModels` 相同的同源闸门，两侧不同源即不再带上本地 Key。
  - **「不含密钥」导出现在也剥离模型卡片级凭据。** 卡片里的 `Headers` / `Options` / `RawCfg`（`internal/core/backup.go`）此前未经过脱敏词表，模型卡片上自定义的鉴权头会随「不含密钥」备份一起外流；现在一并按同一套归一化词表剥离，卡片级凭据不再外泄。
  - **新增的拨号守卫不再拦掉用户自己的本地代理。** `internal/core/dial.go` 改为仅当被拨号的地址恰好就是环境里配置的代理时才放行（否则本地代理用户整体不可用），直连仍逐 IP 复检。副作用是连通性探测（`probe.go`）同样受该守卫约束——未打开「允许本地与内网地址」时，对本地 / Ollama / LAN 网关的探测会失败，这是修掉 SSRF 缺口的代价，需在文档与文案里让用户看得见原因。
  - **更新器改名失败不再误删用户刚下载的暂存文件。** `internal/update/helper.go` 在「暂存文件改名失败」分支不再删除用户刚下载的暂存文件（此前的回滚会把那份唯一副本删掉，只能重新下载），并统一了 Windows 路径的比较口径为大小写不敏感（`strings.EqualFold`，与此前已有的自身路径判定一致）。
  - **`MergeConfig` 在全部选中项都被跳过时不再多余写盘。** 当所有选中提供商都因 Key 与 base_url 不同源而被凭据冲突闸门跳过时，`internal/app/app.go` 不再把内容毫无变化的目标文档重写一遍（只刷新 mtime）。写入前的同级备份与还原点仍由统一的 `prepareChange` 前置动作生成，不受此次判断影响。
  - **前端两处提示计数修正。** 批量档位补全的「补入 N 个档位」此前恒为 0（计数取错集合），现按实际并入的档位数显示；备份导入成功提示的「跳过 N」此前漏计 `key_conflict` 条目，现把凭据冲突的跳过项一并计入。
  - **文档数字与生成表对齐。** 能力目录条数 2303 → 2305、models.dev 快照 1309 → 1311，带能力位的规则数 47/84 → 82/84、44/54 → 45/54（`CHANGELOG.md`、`TODO.md`、`MODEL-MANAGEMENT-PLAN.md`）。

- **安全审计遗留项整体修复（2026-10-15 两路只读审计的第 22 项清单）。** 逐条落地如下：
  - **「不含密钥」的备份不再把本地真实 API Key 挂到文件里的 baseUrl 上（凭据外泄）。** `ImportBackup` 与 `MergeConfig` 原先在「条目已存在 + 文件无 Key」时无条件沿用本地 Key，同时采用文件里的 `base_url`——一份可分享的无密钥备份只要把 baseUrl 换成别的主机，用户下次取模型/测试连接就会把真实 Key 发给那个主机。现在**只在两侧同源**（`core.SameURLOrigin`：协议 + 主机 + 端口，大小写与根点归一）时沿用本地 Key；来源不同则**整条跳过**（不改 baseUrl、不动 Key），并在响应里用 `key_conflict` 列出跳过项与两个地址，界面直接提示「先删除本地同名提供商再导入」。`InspectBackup` 的预览此前也不显示 base_url（用户看不出目标域名被换掉），现已补上，且在导入弹窗里单独一行显示。同族缺陷 `RefreshProviderModels` 的 `baseURLOverride` 同样处理：覆盖主机与已存配置不同源时不再把已存 Key 发过去（界面上的重新获取本来就带表单里的 Key，正常路径无感）。
  - **SSRF：出站请求补上「解析后 IP 复检」。** `urlutil` 的地址校验只看得到 URL 里的主机名，域名可以先通过校验再解析到内网（DNS rebinding）。`fetch.go` 与 `probe.go` 现在都走新的 `core.guardedDialContext`（语义与 `internal/update` 的 `safeDialContext` 一致，core 不反向依赖 update 故保留同口径实现）：建连前逐 IP 复核，命中私有/保留地址即拒；用户显式打开「允许本地与内网地址」时放行（否则本机测试服务器都请求不了）。同时补齐判定本身的漏判：绝对形式主机名（`localhost.`）与 `%` zone 先归一，并按段拒绝标准库 `IsPrivate`/`IsLoopback` 不覆盖的保留地址（运营商级 NAT `100.64/10`、TEST-NET 三段、`192.0.0.0/24`、6to4 中继、`198.18/15`、`240/4` 含广播、组播，以及 IPv6 的 `2001:db8::/32`、`2002::/16`、`64:ff9b::/96`、`100::/64`、`.local`）。
  - **更新 helper 从「任意 exe 替换 + 启动」原语收紧为只能自我替换。** `ValidateInstallRequest` 现在要求暂存文件必须是更新器自己的下载产物（同目录、`.apm-update-*.exe`），`RunHelper` 再用 `os.Executable()` 确认 target 就是「正在运行的自己」——helper 本来就是以目标可执行文件启动的，所以这条等式成立时，调用方已经能在目标目录写文件（那已是完全控制）。父进程 PID/镜像名在 Windows 上无法可靠校验，故不依赖（代码注释写明理由）。
  - **关闭「校验 → 替换」之间的 TOCTOU，并让回滚失败可见。** 校验时用**打开句柄**的 Stat 取文件身份（路径式 `os.Stat` 返回的身份是零值），替换后先比对目标路径的文件身份、再按目标路径复算一次摘要：落到目标路径上的必须是刚校验过的那份内容，中途被换文件或原地改写都会被拦下并回滚。句柄不能在替换期间一直持有——Go 的 `os.Open` 在 Windows 上只共享 `READ|WRITE`，持有它会让紧随其后的改名直接失败；而 `os.SameFile` 又是按记住的路径重新打开文件补身份的，暂存路径改名后不复存在、会误判成「不同文件」。这两条平台行为都由新增的替换路径用例锁住（能真实跑完「备份 → 改名 → 身份复检 → 摘要复算 → 启动失败 → 回滚」全流程）。回滚不再是「忽略所有错误」：每一步失败都会汇总进返回给用户的中文错误，并提示手动恢复备份路径。启动时新增 `CleanupStaleStaged` 清理下载中途退出留下的 `.apm-update-*.exe`（只清同目录、更新器前缀、`.exe` 且超过 1 小时的文件，避免误删并发实例正在进行的下载）。
  - **`StripBackupKeys` 递归 + 忽略大小写。** 原先只删 `options_json` 顶层的 `apiKey`/`api_key`，`headers` 里的 `Authorization`/`X-Api-Key` 会随「不含密钥」备份一起外流。现在按归一化键名（小写、去 `-`/`_`/空格）递归遍历对象与数组，命中 `apiKey`/`x-api-key`/`authorization`/`auth`/`token`/`access_token`/`secret`/`client_secret`/`password`/`credential` 等词表即删；无法解析的 `options_json` 仍整个丢弃（宁缺勿漏）。
  - **备份解密的 PBKDF2 迭代数加上界。** 迭代数完全来自文件，构造一份 `iters` 极大的备份即可让应用长时间卡在密钥派生上（本地拒绝服务）。现在超过 `backupPBKDF2ItersMax`（当前值的 10 倍，600 万）直接返回新的 `ErrBackupUnsupportedParams`，与「密码错误」区分开。
  - **其余低危项：** 备份导出/导入与查看一律拒绝 UNC 与设备路径（`\\server\share`、`\\?\`，备份可能含密钥，不该写去网络共享）；「安装并重启」增加**一次性确认令牌**（`PrepareUpdateInstall` 发放、`InstallUpdate` 校验后立即作废，5 分钟过期，界面上只有显式确认弹窗会领牌）；`SetKeychainLink` 写入前确认钥匙串条目真实存在（否则界面显示「已关联但填不出 Key」）；迁移预览的原文一律 `RedactSensitive`（预览渲染到界面，没必要带明文密钥，写入路径照旧使用完整配置）；DSH profile 写出口不再留下旧条目的明文 `apiKey`（凭据只经 `apiKeyEnv` 落到 `.credentials.yaml`，此前「清空 Key + 已有 apiKeyEnv」会在 patch 里留下旧明文）。
  - **两项经复核后有意不改，理由记在 `TODO.md` 第 22 项：** ① 前端 `call()` 未把 `success:false` 全局归一化为 rejection——该不变量需逐调用点评审（多数调用点自行判 `success` 并展示错误），全局抛异常会改变一批既有错误处理的形态，风险大于收益，留待单独一轮；② DSH 0.1 legacy 顶层 `provider` 布局仍写明文 Key——0.1 是否支持 `apiKeyEnv` 未实证（无本地 0.1 环境），贸然改成引用式可能让 legacy 用户彻底无法鉴权，等有实证再改。另有 **三处审计表述经实测更正**：整型/八进制/十六进制 IP 形式（`2130706433`、`0x7f.0.0.1`）在 Windows 上既不被 `net.ParseIP` 解析也不被系统解析器解析，无实际可利用性（真正的绕过是 `localhost.`）；钥匙串「用于导入」的 `fillApiKey` 消费后**已被清理**，未清理的是 `zcode-pm:manualRefresh`（本次修掉）；`AGENTS.md` 的后缀词表早已含 `.snapshot_<ts>`，缺的只是 `.tmp_*`（已补）。

- **provider 自报的档位枚举现在也算「支持思考」（2026-10-15，阶段 F）。** `BuildModelCardsForSite` 原本先按能力表/关键词算 `reasoning`、再算 `levels`（provider 优先）。于是当 provider 在 `/models` 里自报了档位、而模型 id 既不在离线目录/内建正则里、名字也不命中关键词表时，卡片会同时带着 `reasoning=false` 与 `_supported_levels=["low","high"]`——「列出了可选档位却声明不支持思考」，前端据此禁用思考开关，档位补全也会把它当成用户的显式关闭而跳过。现在 provider **自报**档位含非 off 档且 provider 未显式声明思考支持态时，`reasoning` 补正为 `true`；判据抽成 `infer.go` 的 `hasNonOffLevel`，与能力表推断同口径（provider 显式说了不支持时绝不覆盖，只有 off 的枚举也不算支持）。
- **`_supported_levels` 曾被「导入时自动补全上下文与输出长度」开关牵连（2026-10-15，阶段 F 修正）。** 该开关的文案只管上下文与输出长度这类**会被写进配置的值**，但拉取模型时档位推断也被它门控：关掉开关后卡片既没有「档位自适应」的弱化显示，也没有数据可供「按能力表补全档位」——而同一张卡保存后重载又有（编辑路径本来就不受开关约束），两条路径口径不一致。现在 provider 自报档位仍然优先、能力表档位无条件兜底，与同性质的 `_capabilities_inferred` 一致；**ctx/out 与 `capabilities` 有效值的门控保持不变**（`TestBuildModelCardsForSiteSupportedLevels` 已按新口径改写，并补了「档位照填、长度仍留空」的对照断言与站点层档位的开关无关性用例）。
- **能力推断的「输入模态丢 text」缺陷（2026-10-15 复核修复）。** ZCode 内建 `modelRules` 84 条里只有兜底规则 `.*` 写 `supportsText`，其余 83 条只写 `supportsImage/Video/Audio/Pdf`——text 是默认项、其余模态是叠加的增量。生成脚本原来只收「显式为真」，抽出的输入模态因此丢掉 text（用户两个网关的 90 个模型里 34 个受影响）；又因层间对模态是整字段「前层胜出」，这个残缺集合把 models.dev 的完整集合整段挡住，最终会让 DSH 落出 `input: [image]`（DSH 里等于声明「不接受文本输入」，且自动补全默认开启）。现在生成期对 `inputFormat.supportsText` 采用「**段存在时缺键即真、只有显式 `false` 才排除**」，而 `inputFormat` **整段缺失**不产出模态段（逐键 overlay 里那是「本层不声明输入面」；23 条站点规则 + 2 条家族规则属此类，否则它们会在层序修正后把下层收窄——实测 `api.z.ai/api/anthropic` 等网关各约 98 个 id 丢 image/video）。输出侧刻意**不对称**：`outputFormat.supportsText` 保持严格 `=== true`，因为 ZCode 的输出面只有这一位、表达不了 audio/image 输出，一旦「缺键即真」胜出会把 `mimo-v2.5-tts`、`gpt-6-astra` 这类模型的非文本输出抹成「只输出文本」。`dshCapabilityModalities` 拆成读/写两个口径：**写端**交集非空必含 text（APM 绝不写出一个声明「不支持文本」的集合），**读端**对显式存在的 `input` 忠实照读（手写的 `input:["image"]` 被 APM 保存一次后会放宽成 `["text","image"]`，有意为之，已在代码注释里写明代价）。ZCode v2 读端同样只对 `inputFormat.supportsText` 采用缺键即真（`_apm_capabilities` 内部通道仍是严格三态，不受影响）。
- **求值链层序与 ZCode 相反（2026-10-15 复核修复，asar 实证）。** APM 原本是「家族正则 > 站点规则」，但 ZCode 自己的规则数组是 `[...modelRules, ...modelApiRules, ...providerSiteRules, ...templateModelRules, ...builtinProviderModelRules]`，`resolve()` 顺序 overlay、**后命中者胜**——即站点规则覆盖家族规则。层序已改为 **`provider 自报 > override > site > zcode > presets > catalog`**。修正前实测：`zcode.z.ai` 系列站点规则下 420+ 个目录 id 丢 image+video（如 `alibaba/qwen3-max`）、`api.deepseek.com` 下 `deepseek-v4-flash*` 丢 image、`opencode.ai/zen/go/v1` 下 14 例输出长度与 12 例 ctx/档位与 ZCode 生效值不符（`grok-4.6` out 128000 vs ZCode 的 500000），修正后全部回到 ZCode 口径。锁定旧层序的测试已改名为 `TestLookupCapabilityForSiteSiteBeatsFamilyRule` 并按正确语义重写，另加一条用真实生成表的对照用例（把层序倒回去即变红）。

- **DSH 的 `input` 不再原样搬运模态。** 此前 `dshModelToRaw` 把 `modalities.input` 原样写进 DSH 的 `input`，而 DSH 只接受 `text` / `image` 两个词；一旦模态里含 `video` / `pdf` / `audio`，整个模型条目会被 DSH 的 schema 拒收。现在按 `{text,image}` 求交集（交集为空则整键省略，绝不留空数组），数据源优先卡片的中立能力位、再回落真实键与 DSH 原生字段，因此往返不丢数据。
- **内部键落盘泄漏（三处出口）。** 旧式顶层 `provider` 布局把编辑载荷的 `models` 原样并入文档、`deepSeekBuildEntry` 的合并语义又会保留旧节点里未被本次提交覆盖的键、`ImportOpencodeProviders` 的产物绕过所有 backend 直写——三处都可能把 `_apm_capabilities`（以及既有的 `_dsh_raw` / `_opencode_raw` / `_raw_cfg` 等）写进用户配置。现在前两处分别在写出口剥离，第三处在出口统一剥掉下划线前缀键（模型级走 `stripAPMInternalModelKeys`，提供商级按前缀剥）。
- **「自动匹配」顺带补能力位。** `ApplyModelPresetsForSite` 用同一次分层求值填 `capabilities`，语义为**只补空缺**（不覆盖用户已逐位确认过的选择）；ctx/out 仍是覆盖式写入，`filled` 仍只统计这两类字段。
- **ZCode v2 的输入模态不再只认 image/video。** `legacyInputFormat` 扩到全部 5 个词，并从 `modalities.output` 推出 `outputFormat.supportsText`。这不是放宽「不推断」纪律：它读的是配置/导入数据里**明确列出**的模态，没有列出的照旧不写键。
 off 档位的原始拼法（`disabled` / `none` / `nothink`）经卡片内部字段 `_off_alias` 原样保真，不再被统一改写导致 ZCode 的 `map` 表达式匹配不到；ZCode v2 的「关闭思考」现在真正持久化——按其真实语义把档位集合收敛为单元素 off 词（此前关闭后重读会翻回开启），并删除 `config.enabled → reasoning.enabled` 的错误映射（`config.enabled` 是模型启用位、与思考无关，仍原样保留不丢数据）；默认档位改为按集合回落（优先 high，否则集合内首个非 off 档），不再因集合不含 `high` 而拒绝保存；DSH 档位放开到全部 7 档，`medium` / `xhigh` 不再在读写中被丢弃，`minimal` 不再被折叠成 `low`；legacy 写盘出口剥离 APM 内部键（`offAlias` 等下划线前缀 meta），不再把它们落进用户 `config.json`。顺带删除死代码 `OpencodeEffortOrder` / `OpencodeEffortToVariant`，前端三处硬编码档位表收敛为单一真源 `frontend/src/constants/variants.ts`。
- `go test` 不再污染真实还原点目录：`internal/core` 里的 `TestZCodeToDeepSeek...`、`TestDeepSeekToZCode...` 与三个 `ApplyLegacyImport` 测试此前只沙箱化了 `DSH_HOME`，把迁移/导入快照写进了 `%LOCALAPPDATA%\AgentProviderManager\restorepoints`。现在它们把 `LOCALAPPDATA`（以及 `USERPROFILE`，并为反向迁移的目标存储列表清空 `ZCODE_PERSONAL_PROVIDER_CONFIG_FILE`）固定到 `t.TempDir()`，与 `setupMigrateTest` 一致。
- **「自动匹配」与拉取模型时的自动补全口径统一。** `ApplyModelPresets`（管理页/导入页的「自动匹配」按钮）此前只查手维护的 `modelPresets` 前缀表，而 `BuildModelCards`（拉取模型时的自动补全）走能力表（APM 覆盖层 > ZCode 内建 84 条规则 > presets 兜底），于是同一个模型在两条路径上会得到不同上限：`deepseek-v4.1-flash` 这类只在能力表里能命中的 ID，拉取时能补上 1000000 / 384000，点「自动匹配」却一格不填（内建规则 `.*deepseek-v4[.-]1-flash` 与覆盖层本都有值，只是 presets 表的前缀匹配在 `.1` 处断裂）。现在两条路径共用能力表；`filled` 仍只统计上下文/输出两类字段（档位补全由卡片自适应负责），覆盖用户已填值前照旧走前端确认。`LookupModelLimits` 保持「只查预设表」不动——它是能力表的兜底层，改它会递归。回归用例 `TestApplyModelPresetsUsesCapabilityTable` 锁定该口径，并前置断言该 ID 不在 presets 表内，避免前提被后人改动后用例悄悄失去区分力。
- **手维护 limits 表排在社区目录之前（2026-10-15）。** 新增社区离线目录时就遇到过排序陷阱：若让社区数据压过手维护的 `presets` 表，会静默改写 **270 个字段 / 180 个模型** 的既有值，其中方向明显错误的包括 `gpt-4` 上下文 128000→8192、`gemini-2.5-flash-tts` 1048576→8192、`claude-sonnet-4-5` 200000→1000000。社区库是第三方聚合数据、且按 provider 变体给出不同上限（`gpt-5.4` 只有 272000，而 ZCode 内建与网关自报都是 1050000），因此固定为「内建与手维护表在前、社区目录只做最后兜底」。目录仍能逐字段补上 presets 从不提供的档位（旧表结构里没有档位列），因此覆盖面不损失；一条回归测试锁定「旧两层已给出的字段在新五层里逐字段不变」，让「只增加命中率」成为按构造成立的性质而非约定。

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
