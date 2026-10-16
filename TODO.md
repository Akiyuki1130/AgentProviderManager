# TODO — Agent Provider Manager 功能补齐清单

> 来源：2026-10-13 Computer Use 全页面实测（v2.0.8）+ 源码功能梳理。
> 按重要级排序，1 为最高。
> 注：条目编号是当时的重要性排名，与实施顺序无关；已完成项会打勾并注明日期。

## P0 — 实测发现的真实缺陷

- [x] **1. 修复 `go test` 污染还原点列表**（2026-10-13 完成）
  还原点页出现大量指向 `AppData\Local\Temp\TestXxx...` 的测试条目（跨 Agent 迁移/旧版导入，时间戳与今早跑测试吻合）。
  根源：`internal/core/dsh_regression_test.go:253`、`TestDeepSeekToZCode...`、`TestApplyLegacyImport` 等只沙箱化了 `DSH_HOME`，但 `MigrateExecuteAtPaths` 等写盘快照落进真实 `RestorePointsDir()`。
  危害：① 挤占 50 个/64MB prune 上限，可能顶掉用户真实还原点；② 条目带「恢复」按钮，误点会用测试数据覆盖真实配置。
  修法：相关测试补 `t.Setenv("LOCALAPPDATA", ...)`（或 USERPROFILE 沙箱）；另考虑一次性清理存量孤立条目。
  处理：5 个测试已补沙箱（`dsh_regression_test.go` 两处、`zcode_v2_test.go` 三处；`app_cross_agent_migrate_test.go` 经 `sandboxApp()` 本已隔离），全量 `go test ./...` 通过且不再新增条目；55 个存量测试条目已按 `target_path` 含 `\Temp\Test*` 逐条校验后删除（真实条目为 0 个）。
  说明：本清单编号按当时的重要性排名，与实施顺序无关。

## P1 — 高价值功能（2–5 已于 2026-10-13 完成）

- [x] **2. 独立「测试连接」按钮**
  目前验证 Base URL+Key 只能靠「重新获取模型」兼任，且只探 `/v1/models`，不验证 chat 端点与鉴权实际可用。
  方案：轻量探测（models 可达 + 1-token chat 请求 + 延迟），错误分类（DNS/证书/401/404/超时）。
  完成：`core.TestConnection` 两段探测（models 复用既有取模型链路 + 按 kind 的真实对话端点 1-token 请求），绑定 `TestConnectionRaw` / `TestProviderConnection`，管理页与导入页内联显示逐段耗时与分类错误。未做“证书”细分（沿用 `ModelFetchError` 的 CONN/TIMEOUT/HTTP_xxx/BAD_URL 分类）。

- [x] **3. 钥匙串与提供商打通**
  钥匙串目前是孤立保险箱，提供商 API Key 不走它。
  方案：API Key 字段支持引用钥匙串条目（如 `${keychain:id}`）；新建/导入向导加「存入钥匙串」；避免明文 Key 落 provider_config.json。
  完成：编辑页/导入页可「从钥匙串」填入、「存入钥匙串」保存并关联；钥匙串页显示每条目被多少提供商使用。
  偏差：ZCode v2 严格 schema 不接受 `${keychain:id}` 之类的间接引用（会让 ZCode 自己读不了配置），因此关联只记在 APM 的 settings.json，配置文件仍写入真实值——harness 侧无法避免明文，这是其自身格式约束。

- [x] **4. 提供商列表搜索/排序**
  模型区有搜索框，提供商侧栏没有；提供商多时难翻。
  方案：过滤框 + 按名称/模型数/最近使用排序。
  完成：侧栏加搜索框 + 排序下拉（默认 / 名称 / 模型数），计数显示 filtered/total。未做“最近使用”排序（无使用时间数据源）。

- [x] **5. 配置导出/备份包**
  现在只有还原点和剪贴板复制，没有「导出 JSON（可选脱敏 Key）」「导出可分享的提供商配置」。
  跨机器迁移/备份场景刚需。
  完成：`apm-backup v1` 信封导出（全部/指定提供商；是否含密钥与加密方式正交：明文[勾选密钥时二次确认] / 默认加密[内置种子 AES-256-GCM，跨设备可解、防明文直读] / 密码加密[PBKDF2-SHA256 + AES-256-GCM]）+ 导入预览与冲突策略，导入自动建还原点。
  范围：只导出提供商配置；钥匙串条目、settings.json、还原点不进包。初版默认加密为 DPAPI 本机模式，按需求改为内置种子跨设备模式（2026-10-13）。

## P2 — 体验与安全

- [x] **17. 模型能力/档位/厂商三阶段改造**（阶段 A + B + C + D 全部完成 2026-10-14）
  详见 `MODEL-MANAGEMENT-PLAN.md`（v6）。四阶段全部完成：A 修档位缺陷 → B 模型能力表地基 → C 档位自适应 + 高级开关 → D 厂商分组视图。
  - **阶段 A 已完成**：`canonicalVariant` 取消 `minimal→low` 折叠（A-6 核心）；off 词原拼法经新增卡片字段 `_off_alias` 往返保真（A-1/A-8）；`CardToCfg` 默认档位按集合回落、不再因集合不含 `high` 报错（A-2）；关闭思考语义（A-7，已实测确认 `config.enabled` 是模型启用位而非思考位）；DSH 档位放开到 7 档、不再丢 `medium`/`xhigh`（A-6）；前端三处硬编码档位表收敛为单一真源 `frontend/src/constants/variants.ts`（A-3）；ZCode 侧默认档位控件加「仅对 DSH 生效」提示（A-4）。
  - **独立验证轮**（三个只读 Agent：需求符合性 / 代码审查 / 改进点）发现并修复了一个**本改动引入的阻断回退 S1**：
    - **S1**：ZCode v2「关闭思考」不再持久化（关闭后重读开关翻回开启）。两重根因：`CardToCfg` 关闭时仍写完整档位集合；`v2ModelCard` 把 `config.enabled` 误映射进 `reasoning.enabled` 导致读回被布尔覆盖。**修法**：关闭时集合收敛为 `["off"]`（ZCode 真实语义 = 档位集合选中 off 词）；删除 `config.enabled → reasoning.enabled` 映射（`config.enabled` 仍由 merge 原样保留，不丢数据）；档位集合不含 off 词的模型（`glm-5.3`、`kimi-k2.7-code`）思考开关禁用并提示。
    - **M1**：默认档位策略三处不一致 → 统一为 high 优先（`DefaultVariantPreference` 改序、删死项 `none`）。
    - **M2**：内部键 `offAlias` 会经 legacy 写入路径落进 ZCode `config.json` → 在 legacy 写盘出口剥离内部下划线键。
    - **M3**：`enabled` 档位在前端胶囊不可见/不可删 → 集合含 `enabled` 时并入选项。
    - **M6**：新增文案入 i18n 三语。
    - **死代码**：删 `OpencodeEffortOrder`（零引用）与 `OpencodeEffortToVariant`（已成 no-op）。
  - 验证：`go build/vet/test ./...` 全通过，前端 `npm run build` 通过，`git diff --check` 干净。关键测试 `TestS1CloseThinkingPersistsAcrossV2RoundTrip` 端到端锁定关闭持久化。
  - **阶段 B 已完成（2026-10-14）**：新增 `scripts/sync-model-catalog.mjs`（从 ZCode 自带 `zcode-builtin.json` 抽 `modelRules` 84 条生成 Go 表，带 `--check`）+ `internal/core/model_catalog_gen.go`（84 条 `(?i)^(?:...)$` 预编译正则）+ `internal/core/capability.go`（`LookupModelCapability`：override > 正则全表逐字段覆盖 > presets 兜底；`detectVendor` 19 家族）+ `internal/core/model_overrides.go`（APM 自维护覆盖层）+ `capability_test.go`（14 用例）。
    - **匹配语义三事实**（实测）：ZCode 是**全串锚定 + 大小写不敏感**；**逐字段覆盖**（非取首条命中）；**有意跳过 `Pattern == ".*"` 纯兜底**，避免把未知模型填成 200000（APM 保留「未知留空」契约）。
    - **`deepseek-v4.1-flash` 已可自动补全**（ctx 1000000 / out 384000 / 档位 off,low,high,max）——用户点名的诉求由内建规则 `.*deepseek-v4[.-]1-flash` 直接解决（此前失败纯因 `presets.go` 前缀匹配在 `.1` 处断裂）。
    - **`GuessReasoning` 回归面**：32 个代表模型双向比对，**零 `true→false` 回归**，8 个 `false→true` 修正（`deepseek-v4-flash`、`gpt-5.6-luna`、`glm-4.6`、`glm-5.3-flash`、`kimi-k2.6`、`kimi-k2.7-code` 等过去被误判为不支持思考）。
    - 卡片新增内部字段 `_vendor` / `_supported_levels`（不写盘，已剥离；前端 DTO 已镜像）。
    - 集中验证：`go build/vet`、`go test ./... -count=1`（6 包全绿）、`npm run build`、两个 `--check`、`go mod verify`、`git diff --check` 全部通过。
  - **阶段 C 已完成（2026-10-14）**：档位自适应 + 高级自填开关。
    - **后端**：新增 `internal/core/variant_policy.go`（`SetCustomVariantsAllowed`/`CustomVariantsAllowed`，`sync.RWMutex`+bool）；`NormalizeVariants` 开关打开时跳过白名单（仍去重 + 归一别名 + **限长 `MaxVariantLen=64` + 禁控制字符**）；`CardToCfg` 尊重开关、`CfgToCard` 补填 `Vendor`/`SupportedLevels`；DSH 新增 `dshEffortForWrite`/`dshEffortForRead` 使读/写对称；`app.go` 新增绑定 + `GetOptions()` 聚合 + `Startup()` 同步。
    - **前端**：`ModelCard.vue` 用 `_supported_levels` 自适应高亮支持的档位（`.pill.unsupported` 弱化不支持的、仍可点开）、开思考时用支持集合做种子；新增「允许自填推理档位」高级开关（`OptionsPage` + `setting.ts`（走后端 `trackSetter`）+ `api` + `App.vue` 回读）；自填输入并入集合（归一判重、限长）；`ManagePage`/`ImportPage` 批量档位支持态标记 + `uiConfirm` 提示；i18n 八条新键三语齐全。
    - **C0 决策**：既有 `reasoningAllIntensities` **保留**（语义＝开思考时预置几分档的种子大小），与 C1 自适应、C2 自填三者正交，不合并。
    - **过程补修**：DSH **读端**原本仍按白名单过滤，导致自填档位「写进去、重读就没了」→ 新增 `dshEffortForRead` 并改读端两处，读/写对称；补 `TestDshCustomEffortReadWriteSymmetry` 与 `TestZCodeCustomVariantRoundTripSymmetry`。
    - **独立代码审查轮**（只读 Agent）：**无阻断**。修两条应修——① 自填档位名无长度/字符校验 → 加 `MaxVariantLen=64` + 禁控制字符（前后端同帽）；② 批量提示文案称「仅作用于勾选项」而实现是「作用于可见模型」（**阶段 A 之前就如此**）→ 修正文案使与行为一致。另修 ja 文案混入中文字「档」8 处、`commitCustomVariants` 补等价归一、删无实证的 `@@` 注释。
    - 集中验证：`go build/vet/test ./...`（含 `-shuffle=on`）全绿、`npm run build` OK、两个 `--check` OK、`go mod verify`、`git diff --check` 全部通过。
  - **阶段 D 已完成（2026-10-14）**：厂商分组视图（纯前端 `ManagePage.vue` + i18n）。
    - 列表头部新增「按厂商分组」开关（`localStorage: zcpm.vendorGroup`）+ 厂商多选筛选（选项由当前模型集动态生成、带计数、`unknown` 排最后）。
    - `filteredCards` 仍为**扁平数组**（搜索 → 厂商筛选），既有批量逻辑作用域不变；分组只在渲染层（`groupedCards`），`TransitionGroup` 仍只保留在最外层避免嵌套 FLIP 动画错乱。
    - 分组头显示厂商显示名 + 模型数，用既有 token、无新配色；**不写盘**（分组/筛选纯视图状态）。
    - i18n 新增 22 键三语齐全、无 emoji。
    - 验证：`npm run build`、`vue-tsc --noEmit`、`go test`、`git diff --check`、`wails build` 端到端全通过，并确认标记进入 `frontend/dist` 产物。
    - 说明：原计划的阶段 D Sub Agent 因**账户余额不足**失败（未产生改动），由主会话直接实现；未做真实窗口的视觉截图核对。
    - **后续补齐（2026-10-14，提交 `af68a1e`）**：导入页模型列表也接入同一开关与厂商多选筛选；逻辑与外观各收敛为一处（`composables/useVendorGroup.ts` + `global.css` 的 `.vendor-group*`），两页不再各写一份；列表整体换掉后自动收敛已消失的筛选项；手动添加的模型经既有 `BuildSingleCard` 补 `_vendor` / `_supported_levels`（只取内部 meta，不填上下文与输出）。
  - **已提交**：阶段 A + B + C + D 全部改动已于 2026-10-14 随功能提交入库（`3006728` 功能批次、`d04ab90` 文档批次），导入页厂商分组为 `af68a1e`。
  - **需求符合性结论**：#1（自动配置补全）地基已落地（ZCode 正则 + APM 覆盖层；`deepseek-v4.1-flash` 可补全）；#2 已完成（阶段 A 修缺陷 + 阶段 C 自适应档位与高级自填开关）；#3 已完成（阶段 D 厂商分组视图）。**三项原始需求全部落地。**

- [~] **18. 补齐模型匹配规则**（2026-10-15 主体完成，剩 a–e 五条尾巴）
  阶段 B 只抽了 ZCode `modelRules` 84 条，覆盖不到的部分模型拿不到正确的 context/output/档位。本轮补齐如下（细节见 `MODEL-MANAGEMENT-PLAN.md` 与 `CHANGELOG.md`）：
  1. **新增两层离线信源**：`scripts/sync-ability-catalog.mjs` → `internal/core/capability_catalog_gen.go`（**2305 条精确 id**：pi-ai 994 + models.dev 1311；pi-ai 侧逐文件 SHA-256 校验 `.manifest.json`；`--check` **完全离线且跨天幂等**，`--update-snapshot` 才联网刷新 `internal/core/data/models-dev-snapshot.json`；`--verify` 与 OpenRouter `/api/v1/models` 交叉校验、只报告不并入）。
  2. **站点层**：`scripts/sync-model-catalog.mjs` 追加产出 `model_site_rules_gen.go`（ZCode `providerSiteRules` 54 条，含 `baseUrlMatch`/`apiTypeMatch`）。匹配语义用 asar 内代码实测确认：**模型名全串锚定 + 大小写不敏感；baseUrl/apiType 全串锚定 + 大小写敏感**；baseUrl 先归一化（摘 search/hash、去尾部 `/`）。有意偏离一处：apiType 传空串视作「未知」而跳过，ZCode 只在 `null` 时跳过（受影响的 6 条 `.*` 规则都不给能力字段）。
  3. **求值链改为五层**（逐字段覆盖）：**provider 自报 > override > site（需 baseUrl/apiType 命中）> ZCode `modelRules` > presets（手维护旧表）> catalog（社区目录兜底）**。`presets` 排在 `catalog` 之前是刻意的：社区库不得静默改写 APM 既有值（实测若反向排序会改写 270 个字段，如 `gpt-4` 128000→8192、`gemini-2.5-flash-tts` 1048576→8192）。新增入口：`LookupModelCapabilityForSite` / `BuildModelCardsForSite` / `CfgToCardForSite` / `ApplyModelPresetsForSite`（旧函数保留并转调，行为不变）。
  4. **拉取模型时读 provider 元数据**（此前只读 5+5 个 token 键，档位全丢）：新增 `metaLevels`（`thinking_level`/`thinkingLevel`/`reasoning_effort(s)`/`supported_efforts`/`reasoning_options[type=effort].values`/`supported_parameters`）与 `metaReasoning`；**provider 给的档位优先于能力表**（`_supported_levels`）；ctx 键补 `max_input_tokens`/`context_size`，out 键补 `output_token_limit`/`max_new_tokens`。
  5. **实测效果**（CMD GOAT 猫猫 / 秋雪，同一网关 `api.commandcode.ai`，90 个模型）：档位 **43/90 → 79/90**；ctx/out **51/90 → 90/90**。中转 `/v1/models` 只自报 `context_length`，既无输出上限也无档位字段，因此这两个 provider 的收益全部来自静态表与覆盖层。
  6. **覆盖层**：`model_overrides.go` 新增 7 条（`meituan/LongCat-2.0`、`nvidia/nemotron-3-ultra-550b-a55b`、`poolside/laguna-s-2.1-free`、`qwen/qwen3.6-max-preview`、`stealth/glyph-cluster:free`、`stepfun/step-3.5-flash`、`tencent/hy3-paid`），每个字段标「同族直取 / 上游自报」；上游自报的读数逐条写在条目注释里，网关自报的脱敏快照（只含 id 与 contextLength）**只作开发者本地存档、不入库**（见 `.gitignore`），本地存在时由 `model_overrides_test.go` 断言一致，不存在时该断言跳过。
  - **剩余（本项未完成部分）**：
    a. **11 个 id 仍无档位枚举**（各源只给「支持思考」布尔，没有档位列表）：`claude-haiku-4-5-20251001`、`inclusionai/ling-3.0-flash-sante:free`、`meituan/LongCat-2.0`、`MiniMaxAI/MiniMax-M2.5`、`MiniMaxAI/MiniMax-M2.7`、`MiniMaxAI/MiniMax-M3`、`Qwen/Qwen3.6-Max-Preview`、`stealth/glyph-cluster:free`、`xiaomi/mimo-v2.6-flash`、`xiaomi/mimo-v2.6-pro`、`xiaomi/mimo-v2.6-pro-ultraspeed`。下一步：查各家官方文档或 ZCode 升级后的 `modelRules`，有实证再写覆盖层，**不编数**。
    b. **`--verify` 报出 96 处与 OpenRouter 不一致**（例：`thinkingmachines/inkling` 表内 1048576/32768 vs OR 524288/471859；`z-ai/glm-5.3-flash` 表内 1048576/131072 vs OR 943718）。需按官方文档逐条裁定后写覆盖层；在裁定前保持「ZCode 内建优先」以免引入回归。
    c. **覆盖层前缀语义**：`Match` 是前缀匹配，`nvidia/nemotron-3-ultra-550b-a55b` 会外溢到 `:free` 兄弟（档位取值刻意取成与之一致，只有 ctx 被一并改写）；此外既有条目 `deepseek-v4.1-flash` 不带命名空间，命中不了网关原拼写 `deepseek/deepseek-v4.1-flash`（数值恰好相同，无实害）。若要把覆盖层写成「精确 id 优先 + 去命名空间容错」，需改 `matchOverride`。
    d. **阶段 4（运行时手动更新能力表）未做**：需要固定白名单域名 + 既有 URL/SSRF 复检，默认关闭，单独评审后再实施。
    e. **覆盖层「上游自报」条目的证据登记是手工的**：新增这类条目若忘了登记来源表与 `model_overrides_test.go` 的断言，测试抓不到（只有注释里的文字要求）。彻底修法是把「来源类别 + 证据 id」提升为 `modelOverride` 的结构字段，由测试断言「所有上游自报型条目都必须有证据」。
  - 影响面：只影响「自动补全命中率」，不改写既有值（新顺序按构造成立，并由回归测试锁定）；不触碰任何写入路径。

- [x] **19. 「自动匹配」与拉取模型时的自动补全口径不一致**（2026-10-14 实现导入页厂商分组时发现，同日修复）
  症状：导入页/管理页的「自动匹配」按钮（`ApplyModelPresets`）只查手维护的 `modelPresets` 前缀表，**不走**阶段 B 的能力表；而拉取模型时的自动补全（`BuildModelCards` → `LookupModelCapability`）两层都走。同一个模型，两条路径给出的上限不一样。
  可复现：`deepseek-v4.1-flash` 不在 `modelPresets` 里（内建规则 `.*deepseek-v4[.-]1-flash` 与覆盖层都有值，只是 presets 的前缀匹配在 `.1` 处断裂）——自动补全能填 ctx 1000000 / out 384000，点「自动匹配」却一格不填。
  修法：`internal/core/presets.go` 的 `ApplyModelPresets` 改用 `LookupModelCapability` 取值，`filled` 仍只统计 context/output；`LookupModelLimits` 保持"只查预设表"不动（它是能力表的兜底层，改会递归）。
  完成：已切换口径；`TestApplyModelPresets` 期望值按源表重推（`grok-4.6` 的 output 128000 来自内建规则，不再是「未公开故为 0」），新增 `TestApplyModelPresetsUsesCapabilityTable` 锁定口径（含"该 ID 不在 presets 表内"的前置断言）。`go test ./... -count=1` 与 `go vet ./...` 全绿。

- [x] **20. 补全模型能力维度（阶段 E，2026-10-15 完成）**
  触发：模型参数里除上下文/输出/档位外，还有一整类能力可记录（是否支持图片/视频/音频/PDF、结构化输出、工具调用…），用户要求「能自动填的全填」，且**不局限于 ZCode v2**——DSH 与后续 harness 都要从同一层取值。方案与实测证据见 `MODEL-MANAGEMENT-PLAN.md` 第 10 节（含冻结契约 10.2.4 与实施记录 10.8）。
  1. **中立层**：新增 `internal/core/model_features.go`（`ModelFeatures`：输入模态 5 位、输出模态、工具调用、结构化输出、原生联网搜索、中段 system、附件、温度；**全部三态，`nil` = 未知而非「不支持」**）与并列生成表 `internal/core/capability_features_gen.go`（**2320 条精确 id**，编码 `<输入模态>|<输出模态>|<flags>`，flags 固定序 `tjwmaT`，fail-closed 解析；既有 `ctx|out|levels` 三段表与它的解析器一字未动）。
  2. **求值链**：override / 站点规则 / ZCode 正则 / presets / 离线目录各层与新增能力位走**同一套逐字段合并**（层间填空缺、层内后命中覆盖），`.*` 兜底规则照旧整条跳过（否则内建兜底的 `supportsToolCall:true` 会占满所有模型）。测试锁定「旧层已给出的位不得被新层改写或清空」（含三态交锋：已知为假不被后层真值覆盖）。
  3. **三 harness 映射（只写真）**：ZCode v2 → `inputFormat.supports*` / `outputFormat.supportsText` / `supportsToolCall` / `supportsJsonSchemaOutput` / `supportsNativeWebSearch` / `supportsMidConversationSystem`；DSH → `input`（收敛 `{text,image}`，交集空则整键省略）+ `compat.supportsStrictMode` / `compat.supportsTemperature`；opencode → `modalities{input,output}` / `attachment` / `tool_call` / `temperature`。绝不写 `null` / `false` / `enabled` / `requiresMfjsToolSchema` / `optionSpecs.*.map`。
  4. **顺带修的两个真缺陷**：① `dshModelToRaw` 原样把 `modalities.input` 搬进 DSH `input`，含 video/pdf/audio 会被 DSH schema 拒收（现按交集收敛）；② DSH 两处写出口（旧式顶层 `provider` 布局、`deepSeekBuildEntry` 的合并路径）会把内部键 `_apm_capabilities` / `_dsh_raw` 落盘（现已统一剥离）。
  5. **数据源加宽**：`sync-model-catalog.mjs` 抽出能力位列（`modelRules` 82/84、`providerSiteRules` 45/54 条带位）；`sync-ability-catalog.mjs` 新增 pi-ai `input[]`+compat 白名单与 models.dev 五个字段，快照重裁 367KB→803KB（155 provider / 3799 条目），同一 id 多来源**取并集**（pi-ai 词表只有 text/image，给不出 pdf 不等于否定）。
  6. **UI**：模型卡展开区「能力」区（两组胶囊多选 + 六个开关 + 「未勾选 = 未知」说明 + 逐位「采纳推断」）+ 两页批量栏一行「能力」（多选 + 「按能力表补全」，只补空缺），共享 `composables/useCapabilityFill.ts`。
  - **本轮不做的**：价格 / 知识截止 / 发布日期 / 开源权重（无 harness 落点，快照也不加这几列）；`requiresMfjsToolSchema`、`optionSpecs.*.map`、`config.enabled`；运行时可更新能力表（仍是第 18 项 d 的阶段 4，单独评审）。
  - **已知残留（非缺陷）**：① 原生联网搜索在两张离线表里恒无数据，只有 ZCode 的 6 条站点规则带该位，因此仅靠精确 id 命中的模型拿不到它（**数据源缺口**，等上游补齐或手工写覆盖层）；② DSH 配置层表达不了工具调用 / 联网搜索 / 中段 system，这三位仍进中立层供展示与其它 harness 使用；③ 前端「取消勾选」只删键（= 未知），清不掉配置里已存在的 `true`——这是「只写真、从不写 false」策略的必然结果；④ 重裁快照带来 `capability_catalog_gen.go` 的 6 行上游漂移（4 行表头 + 2 行改值 + 2 行新增），已在提交说明里注明来源。
  - **2026-10-15 第二轮复核修复（提交前，含 asar 实证）**：探针对账发现 90 个模型里 34 个的输入模态丢 text（根因：ZCode 内建资源里 text 是默认项、83/84 条规则只写非默认模态，而生成脚本只收显式为真；层间整字段胜出又让残缺集合挡住下层完整集合）。四修：① 生成期 `inputFormat.supportsText` 「段存在时缺键即真」，**整段缺失不产出模态段**（23 条站点规则 + 2 条家族规则，否则会在层序修正后收窄下层）；② 输出侧刻意不 implied（会把 TTS 的音频输出抹成文本）；③ **层序修正**为 `provider 自报 > override > site > zcode > presets > catalog`（ZCode 的 `providerSiteRules` 在规则数组里后于 `modelRules` 并被 `resolve()` 逐条 overlay，即站点规则覆盖家族规则；修正前实测 420+ 个 id 在 `zcode.z.ai` 网关下丢 image+video）；④ DSH 读端改忠实照读、写端保留「必含 text」；v2 读端对 `inputFormat.supportsText` 缺键即真。详见 `MODEL-MANAGEMENT-PLAN.md` 10.9。
  - **本轮未做的人工验证（需真实窗口 / 真实网关；`AGENTS.md` 禁止代读真实用户配置）**：① 对 CMD GOAT 猫猫 / 秋雪复测能力覆盖率（此前档位 43/90→79/90、ctx/out 51/90→90/90，能力位是本轮新增的观测项）；② 浅色与深色两套主题下目测模型卡「能力」区与两页批量栏。代码侧已过 `npm run build`（含 `vue-tsc --noEmit`）与 i18n 三语 / 零 emoji 检查。


- [ ] **21. `app.go` 两条通用兜底写入会落盘 APM 内部键**（阶段 E 复核时发现，当前不可达，存量隐患）
  `App.SaveProvider` 与 `App.ImportProvider` 在「目标 Agent 没有注册 backend」时的兜底分支会把 `CardsToCfgMap` 的产物经 `MergeProviderIntoConfig` + `core.WriteConfig` 直接写盘，**不剥离**内部键（`_raw_cfg` / `_off_alias` / `_vendor` / `_supported_levels` / `_apm_capabilities` 都会落进去）。
  最省事的修法：在这两条兜底分支写盘前调一次 `core.stripAPMInternalModelKeys`；更彻底的是让它们也走 `targetStore.Upsert`（会改语义，需单独评审）。
  为什么现在留着：两个可选 Agent（zcode / deepseek）的 `GetBackend` 恒有解，分支当前不可达；它是「将来往 `AllAgents()` 加新 Agent 却忘了注册 backend」时的隐患，与阶段 E 无关（E 只是多了一个会被泄漏的键）。
  （阶段 E 顺手修掉的两处 DSH 出口与 opencode 导入出口已修，见 `MODEL-MANAGEMENT-PLAN.md` 10.8 第 6/10 条。）

- [x] **22. 安全审计遗留（2026-10-15 两路只读审计，同日整体修复；两项有意不改 + 三处表述更正见文末）**
  审计范围：① 与「能力层误读」同类的缺陷；② `AGENTS.md` 安全不变量（SSRF / 重定向 / 脱敏 / 更新器 / 备份 / 密钥 / 写入收口 / 原子性）的落实缺口。以下均已逐行核对代码确认，**已修复**（逐条记录见本节文末的实施记录）：
  1. **`不含密钥` 备份导入会把本地真实 API Key 挂到文件里的 baseUrl 上（凭据外泄）**：`internal/app/app_backup.go:252-261` 在「ID 已存在 + 文件无 Key」时复用本地 Key，同时把文件里的 `base_url` 写进 payload；`InspectBackup` 的预览不含 base_url（`app_backup.go:196-200`），用户在界面上看不出目标域名被换掉；`MergeConfig` 同型（`internal/app/app.go:1881-1901`）。README 又宣传备份「可安全分享」，等于鼓励互传。建议：baseUrl 变化时**不得**复用本地 Key（或强制逐条确认），预览必须显示 base_url。
  2. **SSRF 缺口**：`internal/core/urlutil.go:55-76` 依赖 `net.ParseIP`，整型/八进制/十六进制 IP（`2130706433`、`0x7f.0.0.1`）与 `localhost.` 一律判为「非私有」；`internal/core/fetch.go:62-101`、`internal/core/probe.go:114-133` 的 `http.Client` **没有**解析后 IP 复检（更新器 `internal/update/update.go:428-455` 的 `safeDialContext` 是现成的正确样板）；`SECURITY.md` 宣称拒绝的「保留地址」还缺 CGNAT（`100.64/10`）等段。
  3. **更新 helper 是无认证的「任意 exe 替换 + 启动」原语**：`main.go:29-35` + `internal/update/helper.go:42-68` 只校验路径形状与 64 位 hex digest，不校验 staged 必须是同目录的 `.apm-update-*`、target 必须等于自身 exe、父进程镜像名；`InstallStaged` 最后会启动 target。
  4. **暂存文件「校验 → 替换」之间存在 TOCTOU**（`internal/update/helper.go:251-274` 全按路径操作），且回滚失败分支被忽略、`.apm-update-*.exe` 从不清理（`CleanupStaleBackups` 只清 `.apm-backup-*`）。
  5. **`StripBackupKeys` 不递归、不折叠大小写**（`internal/core/backup.go:62-82`）：`options_json.headers` 里的 `Authorization` / `x-api-key` 会随「不含密钥」备份外流。
  6. **备份解密直接采用文件里的 PBKDF2 迭代数**（`internal/core/backup.go:165-179`）：构造一份 `iters` 极大的备份即可让应用长时间卡死（本地拒绝服务）。
  7. **中低危清单**：DSH 清空 Key 时明文 `apiKey` 可能留在 patch（`internal/core/deepseek_store.go:280-295`）；DSH 0.1 legacy 顶层 `provider` 布局仍写明文 Key（`internal/core/backend_deepseek.go:179-203`，需确认 0.1 是否本就支持 `apiKey`）；钥匙串「用于导入」把明文 Key 放进 `sessionStorage` 且消费后不清理（`frontend/src/pages/KeychainPage.vue:288`、`ImportPage.vue:597-601`）；迁移预览把含明文 Key 的整份配置回给前端渲染（`internal/core/migrate.go:55-88`）；`InstallUpdate` 后端无一次性确认令牌（`internal/app/app.go:334-348`）；备份导入/导出接受 UNC 路径（`app_backup.go:88-92`）；`RefreshProviderModels` 的 `baseURLOverride` 可把已存 Key 发往任意公网主机（`app.go:1403-1455`）；`SetKeychainLink` 不校验条目存在（`app_keychain_link.go:22-38`）；前端 `call()` 未把 `success:false` 统一归一化为 rejection（`frontend/src/api/index.ts:38-44`）；`.snapshot_` 与 `.tmp_*` 不在 `AGENTS.md` 声明的后缀词表内。
  两路审计**已核查且确认无缺口**的面（勿重复投入）：全仓库无 `os/exec`（唯一进程启动是 `internal/update/process_windows.go` 的 `windows.CreateProcess`，参数数组、无 shell）；出站 HTTP 只有 fetch / probe / update 三条路径且都逐跳复检重定向、跨主机剥离 `Authorization`；更新器的主机白名单 + `safeDialContext` 防 rebinding + 稳定 SemVer + 资产挑选 + SHA-256 + 备份回滚 + 重启清理路径校验（只允许同目录 `.apm-backup-` 前缀）；备份信封的 4MB 上限、密码不落盘、GCM tag 失败即拒、`InspectBackup` 解锁前不吐元数据；provider/模型 id 从不拼进路径；还原点 ID 受正则约束；前端无 `v-html` / `innerHTML`，localStorage 无密钥。
  **实施记录（2026-10-15）**：逐条修复，语义与理由详见 `CHANGELOG.md` 2.1.1-pre「修复」首条。
  1. **备份/MergeConfig 的跨来源凭据复用已封死。** 新增 `core.SameURLOrigin`（协议 + 主机 + 端口，大小写与根点归一，解析失败按不同源）；`ImportBackup` 与 `MergeConfig` 只在同源时沿用本地 Key，来源不同就**整条跳过**并在响应里用 `key_conflict` 报出（含两侧地址），界面按危险警示展示并提示「先删除本地同名提供商」。`InspectBackup` 预览补 `base_url`，导入弹窗单独一行显示（`types/index.ts` 增 `KeyConflict` 与 `base_url`）。**为什么选择跳过而不是「不带 Key 导入」**：v2 的 provider overlay 是「省略 = 保留」，不带 Key 导入会连 baseUrl 一起按文件改掉、把本地 Key 留在配置里指向新域名——下次取模型照样发给对方，等于把漏洞推迟一步。
  2. **SSRF 拨号守卫。** 新增 `internal/core/dial.go`（`guardedDialContext` + `newGuardedClient`，与 `internal/update` 的 `safeDialContext` 同口径的独立实现——core 不反向依赖 update），`fetch.go` / `probe.go` 的 client 改用它；`PrivateHostsAllowed()` 打开时放行（测试与真实内网场景都依赖这点）。`urlutil.go` 归一化 `%` zone 与根点（`localhost.`）、新增 `isPrivateOrReservedIP` 按段拒绝标准库漏掉的保留地址。测试：`audit_url_test.go` 增 `TestIsPrivateOrReservedHostCoverage`（29 个应拦 + 5 个应放行）、`TestValidateFetchURLBlocksAbsoluteHostName`、`TestSameURLOrigin`。
  3. **helper 只能自我替换。** `ValidateInstallRequest` 增两条：暂存文件必须与目标同目录、且基名以 `.apm-update-` 开头；`RunHelper` 增 `ValidateHelperSelfTarget`（target 必须等于 `os.Executable()`）。父进程镜像名不校验（Windows 上不可靠，且等式成立时攻击者已能写目标目录），理由写在函数注释里。
  4. **TOCTOU + 回滚 + 暂存清理。** 新增 `verifyStaged`（用**打开句柄**的 Stat 取文件身份——路径式 `os.Stat` 的身份是零值）、`hashFileContents`（摘要逻辑单点），替换路径抽成 `replaceVerifiedExecutable`（便于测试直接跑）：替换后先 `os.SameFile` 复检目标路径的文件身份、再按目标路径复算摘要，两者都不通过就不启动。句柄不能持有到替换之后（Go 的 `os.Open` 在 Windows 上只共享 `READ|WRITE`，持有即让改名失败 `ERROR_SHARING_VIOLATION`；`os.SameFile` 又是按路径重开补身份的，暂存路径改名后不存在会误判）——两条平台行为都由用例锁住。`rollback()` 改为返回错误并逐项汇总，改名/复检/启动三类失败都会把「回滚失败 + 手动恢复路径」报给用户；新增 `CleanupStaleStaged`（启动时清同目录 `.apm-update-*.exe`，年龄门槛 1 小时）并在 `main.go` 调用。测试：`helper_test.go` 增 4 个（请求形状、拒绝外来 target 且不动文件、清理只删过期项、替换路径全流程含「换文件必须被拦下」）。
  5. **`StripBackupKeys` 递归 + 大小写/分隔符归一。** 归一化键名后按敏感词表递归遍历对象与数组；无法解析的 `options_json` 仍整个丢弃。测试：`TestStripBackupKeysRecursesAndFoldsCase`（`headers.Authorization` / `X-Api-Key` / 深层 `access_token` 必须删、`x-custom` / `note` / `baseURL` 必须留）。
  6. **PBKDF2 迭代数上界**：`backupPBKDF2ItersMax = 6_000_000`（当前值的 10 倍），超范围返回新错误 `ErrBackupUnsupportedParams`。测试：`TestOpenBackupRejectsOversizedPBKDF2Iters`。
  7. **中低危清单**：UNC/设备路径在导出、查看、导入三处一律拒绝（`rejectNonLocalPath`）；`InstallUpdate` 改为消费 `PrepareUpdateInstall` 发放的一次性令牌（16 字节随机、5 分钟过期、用错一次即作废），前端 `stores/update.ts` 先领牌再安装；`SetKeychainLink` 校验条目存在（`app_keychain_link_test.go` 已改为先建条目，并补「不存在的条目」用例）；`readRawText` 的预览文本统一 `RedactSensitive`（写入路径不受影响）；`deepSeekBuildEntry` 无条件 `delete(entry, "apiKey")`（清空 Key 且已有 `apiKeyEnv` 时不再留旧明文）；`RefreshProviderModels` 的两个分支都加同源闸门；`AGENTS.md` 后缀词表补 `.tmp_*`；`ImportPage.vue` 读出 `zcode-pm:manualRefresh` 后立即 `removeItem`。
  **两项经复核后有意不改**：① 前端 `call()` 未把 `success:false` 全局归一化为 rejection——`AGENTS.md` 的不变量需逐调用点评审（绝大多数调用点自行判 `success` 并展示错误），全局抛异常会改变一批既有错误处理的形态，风险大于收益，留待单独一轮；② DSH 0.1 legacy 顶层 `provider` 布局仍写明文 Key——0.1 是否支持 `apiKeyEnv` 未实证（本机只有 0.2），改成引用式可能让 legacy 用户彻底无法鉴权，等有实证再改。
  **三处审计表述经实测更正**（勿按旧描述重复投入）：① 整型/八进制/十六进制 IP 形式（`2130706433`、`0x7f.0.0.1`）在 Windows 上既不被 `net.ParseIP` 解析也不被系统解析器解析，无实际可利用性——真正的绕过是 `localhost.`（本次已封）；② 钥匙串「用于导入」的 `zcode-pm:fillApiKey` **消费后已被清理**，未清理的是 `zcode-pm:manualRefresh`；③ `AGENTS.md` 的后缀词表**早已含** `.snapshot_<ts>`，缺的只是 `.tmp_*`。
  **验证**：`go test ./... -count=1`（6 包全绿）、`go vet ./...`、三条 `--check`、`go mod verify`、`npm run build`（含 `vue-tsc --noEmit`）、`wails build`（exe 报 `AgentProviderManager 2.1.1-pre`）、`git diff --check` 全部通过；安全面新增用例 14 个（core 5 / app 4 / update 5），另有两处既有用例按新语义改写（`app_keychain_link_test.go` 改为先建条目、`helper_test.go` 的暂存文件名改为真实前缀）。**待人工**（需真实窗口/网关）：备份导入弹窗的 base_url 行与 `key_conflict` 警示文案在浅色/深色下的观感；更新安装确认流程走一遍（领牌 → 安装）确认无回归。

- [x] **23. 档位（思考等级）批量补全（阶段 F，2026-10-15 完成）**
  方案、候选取舍与实测证据见 `MODEL-MANAGEMENT-PLAN.md` 第 10.10 节。要点：① 两页批量栏新增一行「档位」（多选带计数 + 「按能力表补全档位」，紧跟「能力」行）＋卡片内「采纳推断档位」；② 语义＝**只增不减地并入**：把 `_supported_levels` 里当前集合还没有的档位加进去，从不删除已有档位（集合为空时等价于整组采纳）；跳过「思考未开」「推断里没有非 off 档」「推断档位都已存在」三类卡片（`fillableLevels` 是唯一实现，单卡与批量共用）；③ 后端两处修正：`cards.go` 去掉 `_supported_levels` 的 `autoFill` 门控（开关文案只覆盖上下文/输出长度，且同类推断通道 `_capabilities_inferred` 本就无条件写入）、provider 自报档位含非 off 档时把 `reasoning` 补正为支持（此前会出现「列出档位却声明不支持思考」）；④ 实测基线：90 个网关 id 里有档位推断 79 个，其中 77 个与盲目默认三档 `[off,high,max]` 不同，而默认三档会写进模型并不支持的档位（off 43 / max 43 / high 19 个）。
  残留：① **不清理旧档位名**（并入语义的固有代价：推断里没有而配置里已有的档位会留下，例如那 43 个并不存在的 off / max）——清理需要一个显式的「覆盖为推断集合」动作，见第 24 项；② 新卡片可能拿到不含 off 的集合（场景 A 的 43 张，其中 19 张源于 ZCode 内建规则本就无关闭档），此时前端按既有 S1 语义禁用该卡思考开关；③ **站点上下文缺口**（既有存量、被本功能放大）：`BuildSingleCard` 与拉取路径的 apiType 恒空/缺失，手动添加的卡片拿不到「只有站点规则才给档位」的模型（`minimax-m2.7`、`qwen3.8-max`）的推断档位；④ 前后端挑默认档位的序不一致（前端 `pickDefaultVariant` vs 后端 `DefaultVariantPreference`，仅在集合不含 high 时分歧）；⑤ 批量补全无撤销（见下方第 9 项）。
  **实施记录（2026-10-15）**：① 后端 `cards.go` 去掉 `autoFill` 门控 + provider 自报档位补正 `reasoning`（`infer.go` 抽 `hasNonOffLevel` 共用口径）；锁定旧口径的 `TestBuildModelCardsForSiteSupportedLevels` 按本文档既有纪律**改写为表驱动**并补两组断言，`TestBuildModelCardsForSiteReasoning` 补四组（provider 列出非 off 档 → 支持；只有 off → 不支持；显式 `reasoning:false` 不被覆盖；autoFill 关也成立），未另开平行测试文件。② 前端新增唯一实现 `composables/useLevelFill.ts`（导出 `inferredLevels` / `fillableLevels` / `applyLevels` / `useLevelFill`；档位名归一抽到 `constants/variants.ts` 的 `canonicalVariant`，与 `ModelCard.vue` 自填档位共用）＋ i18n `level.*` 三语 10 键。③ 三处 UI：模型卡单卡「采纳推断档位」、管理页与导入页批量栏新增「档位」行。④ 已过：`go test ./...`、`go vet ./...`、三条 `--check`、`npm run build`（含 `vue-tsc --noEmit`）、`wails build`。⑤ 独立只读审计轮（Agent）结论：可安全落地，无一处会写出坏配置；采纳其中两条（语义说明与事实相反的口径、provider 自报档位的闸门）。⑥ **待人工**（需真实窗口/网关）：浅色/深色主题目测批量行与单卡按钮；补全后写出配置的 `values` 恰为并入结果且无新增 `false`/`null`；无 off 档模型的思考开关变灰且悬浮提示正确；保存→重载后 `variants`/`default_variant` 与补全结果一致（DSH 侧 `reasoningEfforts` 非空且不为 `false`）；补全后自动保存确实触发。
  **修订记录（2026-10-16，人工验证反馈，方案见 `MODEL-MANAGEMENT-PLAN.md` 10.11）**：人工目测后给出三条反馈，全部采纳：① **删掉「选择要补全的档位」多选**——它的标签承诺「只补这些档位」，实际写入永远是整组集合（`当前集合 ∪ 缺失档位`），选择只改变「处理哪些卡片」，无法表达任何用户意图；补全改为**无参数的一次性动作**。② **补全/覆盖并入「推理档位」那一行**（紧跟胶囊组、在「默认档位」之前），批量栏不再有独立的「档位」行——此前第 2 行与最后一行都在写 `card.variants`，两个名字、上下两处。③ **模型卡的「推理档位 / 自填档位 / 默认档位」三个等重高亮块合并为一个**（三者编辑同一组字段，主次不分）。**能力位的多选保留**：能力是多个彼此独立的位，"只补输入模态、先不确认布尔位"可以精确表达；差异来自**字段形状**（多个离散位 vs 单一整体集合字段），不是实现偷懒——别再为了一致性去删它。另修两处体验：一个都处理不了时说出原因（「N 个模型尚未开启思考…」），「采纳推断档位」的悬浮提示列出本次会并入的档位。i18n 删 2 键（`level.batchBitsPlaceholder` / `level.batchArrayNote`）、改 2 键、增 19 键。**已过** `npm run build`（含 `vue-tsc --noEmit`）与 i18n 自检（475 键三语齐全、引用键零缺失、零 emoji）；**无后端改动、不触碰写盘路径**。上面 ⑥ 的「待人工」项仍然有效，并新增「覆盖的确认明细与实际删除一致」「覆盖写盘后确实生成还原点」两条。

- [x] **24. 「用推断档位覆盖现有档位」（2026-10-16 完成，作为第 23 项的「覆盖」方向）**
  第 23 项的并入语义只增不减，因此清不掉配置里那些**推断集合没有、却被 APM 早期盲目写过**的档位名（实测用户两个网关 90 个模型里 43 个带并不存在的 `off` / `max`）。现已落地为第 23 项那组档位动作里的**第二个方向**（不是独立按钮）：
  - **批量**：批量栏「按能力表补全档位」按钮后的复选框「覆盖已有档位」（默认不勾选、不持久化——破坏性方向不该被粘住）。勾选后按钮文案变「按能力表覆盖档位」并转 error 配色、计数变「将覆盖 N 个模型」；点击先弹 `uiConfirm`（`danger: true`），明细**聚合**给出「删除档位：off×43、max×43」与「其中 N 个模型的档位集合将不含关闭档，其思考开关会变灰（无法再直接关闭思考）」。
  - **单卡**：「覆盖为推断档位」（`mc-adopt-danger` 配色的胶囊），仅当**确有档位会被删除**时露出——没有多余档位时它与「采纳推断档位」结果相同，同时给两个按钮只会让人犹豫。
  - **判定**：`overwriteLevels`（`composables/useLevelFill.ts`，与 `fillableLevels` **同源不合并**）。共同门控照旧（思考未开、推断集合无非 off 档一律跳过——否则覆盖会把 `variants` 清空或置成 `["off"]`，等于**静默把用户的思考关掉**，比"少补"危险得多）；但判据是「推断集合与当前集合**不相等**」而非「有缺失档位」——复用 `fillableLevels` 会让「无缺失但有多余档位」这类最需要覆盖的卡片（现有 `[off,high,max,medium]`、推断 `[high,max]`）一张都不处理。
  - 删掉 `off` 会让该卡思考开关变灰（S1 语义），实测基线里 19/90 的推断集合本就不含 off；不会留脏数据（`cards.go:749-753` 只在集合确实含 `off` 时才回填 `offAlias`，残留的 `_off_alias` 被写入路径忽略）。
  - **待人工**：确认弹窗明细与实际删除一致；覆盖后保存 → 重载的 `variants` / `default_variant` 与推断集合一致（DSH 侧 `reasoningEfforts` 非空）；覆盖写盘后确实生成了还原点（`PrepareChange` 链路）。方案与接口见 `MODEL-MANAGEMENT-PLAN.md` 10.11。

- [ ] **25. 审查复核实录（2026-10-16，已核实、本轮有意不修）**
  本轮审查逐条核对后确认以下 7 点**已核实但本轮有意不修**，记录现象、证据与取舍，以免后人重复投入。证据以源码行号或实测数据为准。
  1. **输入模态在层间是「整字段前层胜出」，与 ZCode 的逐键 overlay 不一致。** `internal/core/model_features.go:140-145`（层间 `mergeFeatures`）与 `:171-176`（层内 `overlayFeatures`）对模态切片是整字段替换，而 ZCode 自身是逐键 overlay（10.9 记录的 asar 证据 `overlayValue(t,n){return n===void 0?t:n}`）。本轮层序改为 `site > zcode` 后，站点规则会整段压掉家族层独有的模态：实测 `claude-fable-5 @ https://api.z.ai/api/anthropic` → APM `[text,image,video]`，而逐键结果是 `[text,image,video,pdf]`（pdf 丢）；`alibaba/qwen3.8-omni-flash` 同站点丢 audio；四条 `APIType=nil` 的网关站点规则命中离线目录 2320 个 id 中的 **179 个**，每个至少丢一个家族层模态。**不污染 ZCode v2 配置**（写端只写真位、省略=不覆盖，且 ZCode 内建 family 与 providerSite 规则在 ZCode 侧始终参与），影响面是 APM 自己的卡片展示 / `_capabilities_inferred` / 「采纳推断」，以及 DSH / opencode 侧的推断宽度。修法候选：① 模态改「层间并集（只增不减）」，代价是「后层显式 false」无法表达、且要改写 `model_features_test.go` 的既有期望；② 给 `ModelFeatures` 增加「已知为假」三态并让生成表编码显式 `false`，最忠实但编码契约、解析器、三个 harness 写端与全部相关测试都要动。需人工裁定。
  2. **生成期 implied 只要求 `inputFormat`「段存在」，不要求段里有阳性键。** `scripts/sync-model-catalog.mjs:142-146`：段存在时 `supportsText` 缺键即真。实例：`providerSiteRules #49`（`.*qwen3\.7-plus…` @ `https://openrouter\.ai/api`）源里只有 `supportsVideo:false`，被抽成阳性的 `text`（`model_site_rules_gen.go` 中 `Features: "text||------"`），命中后把家族层 `[text,image,video]` 压成 `[text]`（丢 image），而逐键结果应为 `[text,image]`。文档 10.9 的 F2 只声明了「整段缺失」这一类，这个第三态无人覆盖。另需注意：家族侧还有 **36 条** `inputFormat` 对象无任何阳性键的规则同属此类，改判据会一并影响它们，须同轮决定。与第 1 点同源，需一起裁定。
  3. **ZCode v2 上「不支持文本输入」无法表达。** 读端 `zcodeprovider.InputFormatModalities` 对 `supportsText` 是「段存在时缺键即真」，写端 `legacyInputFormat`（`internal/zcodeprovider/convert.go`）只写真位、绝不写 `false`。后果：用户在模型卡上取消勾选「文本」输入并保存后，键被删掉，重载时又被读成「支持文本」→ 取消勾选看起来被回滚。要真正表达不支持需要写 `supportsText:false`，与「只写真、从不写 false」的落盘策略冲突。同类代价在 DSH 侧已记录（手写 `input:["image"]` 保存一次后放宽为 `["text","image"]`，有意为之），v2 侧未论及。需人工裁定。
  4. **`internal/update/update.go` 的 `safeClient` 与 `internal/core/dial.go` 有同样的代理问题（既有存量）。** 两者都 `Clone()` 了 `http.DefaultTransport`（保留 `ProxyFromEnvironment`）且未置 `Proxy`，有代理时拨号期看到的是代理地址。本轮只修了 `internal/core/dial.go`（那是本次 diff 引入的回归）；更新器这份在 HEAD 就存在，未动。
  5. **两处还原写盘绕过 `WriteConfigBytes → guardZCodeV2Doc`（既有存量，与 `AGENTS.md` 字面冲突）。** `internal/core/config.go:509-558` 的 `RestoreBackup` 与 `internal/core/restorepoint.go:601-638` 的 `writeRestoredContent` 都是 `.restore_*` 临时文件 + `os.Rename` 直接覆盖目标。内容来自程序自己写过的文件，且分别有指纹复检（`mtime:size:sha256`）与 `validateRestoreContent` 摘要校验，实际风险低。需要维护者明确「豁免」或「补齐收口」。
  6. **拨号守卫带来的用户可见行为变化（需在文档 / 文案里说明）。** `internal/core/probe.go` 现在也走 `newGuardedClient`，因此未打开「允许本地与内网地址」时，对本地 / Ollama / LAN 网关的连通性探测会失败。这是修掉 SSRF 缺口的代价，但用户侧看不到原因。另：拨号守卫在有代理配置时会放行代理地址本身（否则本地代理用户整体不可用），此时目标主机的私网判定回到 URL 层的逐跳校验。
  7. **本轮经核实判定「不是缺陷、有意为之」的两处，记录以免后人重复投入：** ① 批量覆盖按「与推断集合不相等」计数、而单卡「覆盖为推断档位」只在确有档位会被删除时露出——这是 `MODEL-MANAGEMENT-PLAN.md` 10.11 的刻意设计（两处入口的判据本就不同），不是实现漂移；② DSH 写端 `dshCapabilityModalities` 对未知的 text 写真是有意的放宽（写端非空集合必含 text、读端忠实照读，往返用例已锁）。

- [ ] **26. 迁移到 DSH 后模型没有思考强度：DSH 写入路径用能力表兜底档位（阶段 G，2026-10-16 已裁定，待实施）**
  症状：把 ZCode 的 provider 迁移到 DSH 后，DSH 里选择模型时**没有「推理等级」**（整行不显示）。
  根因：DSH 的档位只有两个来源——模型节点显式声明 `reasoningEfforts`，或**路由 key 与模型 id 同时命中 pi-ai 内置目录**（`dsh-llm-pi-ai`：`resolveModelReasoning` 省略字段即 `reasoning: base?.reasoning ?? false`，而手写路由的 catalog 恒为空）；且 DSH 客户端 UI **故意没有**任何推理档位控件（`dsh-client-ui-settings-models`，`reasoningEfforts` 在客户端代码里零命中），所以 APM 不写，用户在 DSH 里补不回来。APM 侧迁移原样投递源卡片，而 **ZCode 的档位来自它自带规则表，用户自己的 `provider_config.json` 里常常没有 `reasoningLevel.values`**，于是写出去的是一个「完全没有推理信息」的模型。
  修法（用户已裁定）：`dshModelToRaw` 在「卡片无档位且无显式开关」时用 `LookupModelCapabilityForSite` 兜底写出 `reasoningEfforts`；门控跟随既有「自动补全」开关（core 侧策略 + app 三处推送）；只补空缺、绝不写 `reasoningEfforts: false`；覆盖**所有 DSH 写入路径**（不只迁移）。细则、实测表与副作用见 `MODEL-MANAGEMENT-PLAN.md` 10.12。
  副作用：开关打开时，任何一次保存都会给「配置里没有档位的模型」补上档位，即使用户这次没碰那张卡。
  证据：asar 实证 + 探针 `internal/core/zz_dsh_reasoning_probe_test.go`（可重跑，`-run TestDshReasoningProbeFromZCodeCards -v`）。
  确诊法（需人工，本轮未做实证）：在 APM 的 DSH provider 编辑器里看该模型的「思考」开关与档位候选是否为空；或看 `$DSH_HOME/profiles/<desktop|web>/cordis.patch.yml` 里模型节点有没有 `reasoningEfforts`。
  待人工：开关打开 → 保存 → DSH 模型选择器出现「推理等级」行且档位与能力表一致；迁移场景端到端同上（需真实窗口与真实网关）。

- [ ] **27. 自填档位词被白名单静默滤光，连带迁移丢掉整个档位集合（2026-10-16 记录，待独立评估）**
  「允许自填推理档位」关闭（默认）时，读卡会把不在 `ZCodeVariantSet` 白名单里的档位名整条丢掉（`internal/core/cards.go:509`/`:524`）；若一个模型的档位**全是**自填名，卡片就退化成「无推理信息」，迁移到 DSH 后思考与档位一起消失（探针实测：源 `values: ["thinking","ultra"]` → DSH 侧无任何推理键）。
  需要裁定的问题：**读侧**该不该与写侧一样尊重该开关（写侧已由 `dshEffortForWrite` 放开）。改动面：`cards.go` 两处门控 + `tokens.go` 的 `NormalizeVariants`，会同时影响 ZCode / DSH / opencode 三条写盘路径，因此单列一轮。

- [ ] **28. 只有 off 的源档位集合被写成 `reasoningEfforts: false`（2026-10-16 记录，待独立评估）**
  源档位集合只有 `disabled`/`none`（即该模型唯一可表达的状态就是「不思考」）时，DSH 侧落地为 `reasoningEfforts: false`，而 `false` 在 DSH 里是「**显式声明非推理模型**」（`deepseek.go` 的 `dshHasNonOffEffort` 退化分支：DSH 的 zod 不允许只含 off 的字典，必须至少一个非 off 档）。
  需要裁定的问题：这种源语义在 DSH 侧该落成 `false`（非推理模型，语义上接近「只有关闭档」）还是**一个键都不写**（= 未知）。涉及 `deepseek.go:836-843` 的退化分支与「兜底不得冒充用户声明」这条原则，需单列评估。

- [ ] **6. 还原点页筛选与分组**
  按 Agent/操作类型/日期过滤；加「清理孤立条目」按钮（顺带解决 1 的存量污染）；回滚前给差异预览（改了哪个文件、几个提供商）。

- [ ] **7. 跨提供商模型总览**
  同名模型（如 deepseek-v4.1-flash）散在多个提供商里，只有单提供商视角。
  方案：「全部模型」聚合页，核对重复与启用状态。

- [ ] **8. 迁移预览结构化**
  目前是一整块原始 JSON，可读性差。改成「新增/保留/覆盖」提供商卡片对比。

- [ ] **9. 会话内撤销（undo）**
  批量编辑模型属性（档位/长度/全选）没有撤销，误操作只能靠还原点这种重手段。

- [ ] **10. 定时/启动时自动刷新模型列表**
  提供商侧模型增减时给通知；目前只有手动/自动两种拉取。

- [ ] **11. 键盘快捷键 + 命令面板**
  桌面应用没有任何全局快捷键，连搜索都要鼠标点。
  进展（2026-10-14，随 UI 批次落地）：已部分完成——`Ctrl+S` 保存当前提供商（脏态才拦截，保留 HTTP 明文确认门）、提供商搜索框 `Esc` 清空；命令面板（原 B12）按约定不做。剩余：全局快捷键体系与 `Ctrl+K` 命令面板仍未做。

- [x] **15. 新建提供商的自动保存与「重新获取」报错**（2026-10-14 完成）
  症状：新建提供商后直接点「自动/手动重新获取」报「提供商「x」不存在」；根因是 `createProvider` 把未落盘的新提供商当成「已保存基线」，`isDirty` 恒为 false——既让所有离开守卫放行（切卡片/切 Agent/关窗时静默丢失），也让后端按 ID 查不到。
  修法（前后端各一层，互为兜底）：① 前端引入 `isPersisted`，脏判定改为「未落盘 ∨ 与基线不一致」，离开弹窗对未落盘的新提供商改用更重的丢弃警告；② 新增 `useAutoSave`（debounce 1.2s + 最小间隔 5s 合并连续改动 + 单飞 + 静默失败），选项页可关闭，自动写入标记 `_operation=auto_save`，还原点清理优先淘汰这类低价值点；③ 需要「提供商已存在」的操作（自动/手动重新获取、钥匙串关联）先经 `ensurePersisted()` 落盘；④ 后端 `RefreshProviderModels` 在带 Base URL 覆盖时容忍「尚未保存」，无覆盖时维持原报错。
  残留：自动保存仍会让还原点增长（已靠合并与最小间隔压住）；关窗恰逢 debounce 窗口内可能丢最后一次改动。

- [x] **16. 版本号统一为单一真源**（2026-10-14 完成）
  症状：版本号散在 9 个文件（`main.go`/`constants.go`/`wails.json`/`package.json`/`package-lock.json`×2/`package.json.md5`/`AppSidebar.vue`/`README.md`×3），发版要手工逐处改，易漏。
  修法：新增根目录 `VERSION` 作为唯一真源 + `scripts/sync-version.mjs` 一次分发（含 `--check`）；Go 侧改为生成文件 `internal/core/version_gen.go`（`main.go` 引用 `core.AppVersion`，不再写字面量）；前端侧栏改为运行时读 `GetAppInfo`，不再硬编码；`go test ./...` 加 `TestVersionSourcesInSync` 兜底，任何派生文件漂移即失败并打印修法。
  说明：`frontend/package.json.md5` 由 Wails 用来判断是否重装依赖，必须与 `package.json` 字节一致，脚本同步生成。改版本现在只需改 `VERSION` + 跑一次脚本（+ 手工更新 CHANGELOG）。

## P3 — 长期方向

- [ ] **12. OpenCode 决断**
  后端绑定（`ImportOpencode` 等）仍暴露给前端但 UI 已雪藏——要么恢复入口，要么删死代码。

- [ ] **13. 新增一个 harness（Claude Code）**
  `core.GetBackend` 注册表就是扩展点；Claude Code 用户群大，是下一个最自然的目标。

- [ ] **14. 模型区虚拟滚动**
  README 自认接近 2000 模型上限性能下降；实测 13 模型的提供商 a11y 树已 400+ 元素，几百模型时列表页会卡。
