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
  1. **新增两层离线信源**：`scripts/sync-ability-catalog.mjs` → `internal/core/capability_catalog_gen.go`（**2303 条精确 id**：pi-ai 994 + models.dev 1309；pi-ai 侧逐文件 SHA-256 校验 `.manifest.json`；`--check` **完全离线且跨天幂等**，`--update-snapshot` 才联网刷新 `internal/core/data/models-dev-snapshot.json`；`--verify` 与 OpenRouter `/api/v1/models` 交叉校验、只报告不并入）。
  2. **站点层**：`scripts/sync-model-catalog.mjs` 追加产出 `model_site_rules_gen.go`（ZCode `providerSiteRules` 54 条，含 `baseUrlMatch`/`apiTypeMatch`）。匹配语义用 asar 内代码实测确认：**模型名全串锚定 + 大小写不敏感；baseUrl/apiType 全串锚定 + 大小写敏感**；baseUrl 先归一化（摘 search/hash、去尾部 `/`）。有意偏离一处：apiType 传空串视作「未知」而跳过，ZCode 只在 `null` 时跳过（受影响的 6 条 `.*` 规则都不给能力字段）。
  3. **求值链改为五层**（逐字段覆盖）：**provider 自报 > override > ZCode `modelRules` > site（需 baseUrl 命中）> presets（手维护旧表）> catalog（社区目录兜底）**。`presets` 排在 `catalog` 之前是刻意的：社区库不得静默改写 APM 既有值（实测若反向排序会改写 270 个字段，如 `gpt-4` 128000→8192、`gemini-2.5-flash-tts` 1048576→8192）。新增入口：`LookupModelCapabilityForSite` / `BuildModelCardsForSite` / `CfgToCardForSite` / `ApplyModelPresetsForSite`（旧函数保留并转调，行为不变）。
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
  2. **求值链**：override / ZCode 正则 / 站点规则 / presets / 离线目录各层与新增能力位走**同一套逐字段合并**（层间填空缺、层内后命中覆盖），`.*` 兜底规则照旧整条跳过（否则内建兜底的 `supportsToolCall:true` 会占满所有模型）。测试锁定「旧层已给出的位不得被新层改写或清空」（含三态交锋：已知为假不被后层真值覆盖）。
  3. **三 harness 映射（只写真）**：ZCode v2 → `inputFormat.supports*` / `outputFormat.supportsText` / `supportsToolCall` / `supportsJsonSchemaOutput` / `supportsNativeWebSearch` / `supportsMidConversationSystem`；DSH → `input`（收敛 `{text,image}`，交集空则整键省略）+ `compat.supportsStrictMode` / `compat.supportsTemperature`；opencode → `modalities{input,output}` / `attachment` / `tool_call` / `temperature`。绝不写 `null` / `false` / `enabled` / `requiresMfjsToolSchema` / `optionSpecs.*.map`。
  4. **顺带修的两个真缺陷**：① `dshModelToRaw` 原样把 `modalities.input` 搬进 DSH `input`，含 video/pdf/audio 会被 DSH schema 拒收（现按交集收敛）；② DSH 两处写出口（旧式顶层 `provider` 布局、`deepSeekBuildEntry` 的合并路径）会把内部键 `_apm_capabilities` / `_dsh_raw` 落盘（现已统一剥离）。
  5. **数据源加宽**：`sync-model-catalog.mjs` 抽出能力位列（`modelRules` 47/84、`providerSiteRules` 44/54 条带位）；`sync-ability-catalog.mjs` 新增 pi-ai `input[]`+compat 白名单与 models.dev 五个字段，快照重裁 367KB→803KB（155 provider / 3799 条目），同一 id 多来源**取并集**（pi-ai 词表只有 text/image，给不出 pdf 不等于否定）。
  6. **UI**：模型卡展开区「能力」区（两组胶囊多选 + 六个开关 + 「未勾选 = 未知」说明 + 逐位「采纳推断」）+ 两页批量栏一行「能力」（多选 + 「按能力表补全」，只补空缺），共享 `composables/useCapabilityFill.ts`。
  - **本轮不做的**：价格 / 知识截止 / 发布日期 / 开源权重（无 harness 落点，快照也不加这几列）；`requiresMfjsToolSchema`、`optionSpecs.*.map`、`config.enabled`；运行时可更新能力表（仍是第 18 项 d 的阶段 4，单独评审）。
  - **已知残留（非缺陷）**：① 原生联网搜索在两张离线表里恒无数据，只有 ZCode 的 6 条站点规则带该位，因此仅靠精确 id 命中的模型拿不到它（**数据源缺口**，等上游补齐或手工写覆盖层）；② DSH 配置层表达不了工具调用 / 联网搜索 / 中段 system，这三位仍进中立层供展示与其它 harness 使用；③ 前端「取消勾选」只删键（= 未知），清不掉配置里已存在的 `true`——这是「只写真、从不写 false」策略的必然结果；④ 重裁快照带来 `capability_catalog_gen.go` 的 6 行上游漂移（4 行表头 + 2 行改值 + 2 行新增），已在提交说明里注明来源。
  - **本轮未做的人工验证（需真实窗口 / 真实网关；`AGENTS.md` 禁止代读真实用户配置）**：① 对 CMD GOAT 猫猫 / 秋雪复测能力覆盖率（此前档位 43/90→79/90、ctx/out 51/90→90/90，能力位是本轮新增的观测项）；② 浅色与深色两套主题下目测模型卡「能力」区与两页批量栏。代码侧已过 `npm run build`（含 `vue-tsc --noEmit`）与 i18n 三语 / 零 emoji 检查。


- [ ] **21. `app.go` 两条通用兜底写入会落盘 APM 内部键**（阶段 E 复核时发现，当前不可达，存量隐患）
  `App.SaveProvider` 与 `App.ImportProvider` 在「目标 Agent 没有注册 backend」时的兜底分支会把 `CardsToCfgMap` 的产物经 `MergeProviderIntoConfig` + `core.WriteConfig` 直接写盘，**不剥离**内部键（`_raw_cfg` / `_off_alias` / `_vendor` / `_supported_levels` / `_apm_capabilities` 都会落进去）。
  最省事的修法：在这两条兜底分支写盘前调一次 `core.stripAPMInternalModelKeys`；更彻底的是让它们也走 `targetStore.Upsert`（会改语义，需单独评审）。
  为什么现在留着：两个可选 Agent（zcode / deepseek）的 `GetBackend` 恒有解，分支当前不可达；它是「将来往 `AllAgents()` 加新 Agent 却忘了注册 backend」时的隐患，与阶段 E 无关（E 只是多了一个会被泄漏的键）。
  （阶段 E 顺手修掉的两处 DSH 出口与 opencode 导入出口已修，见 `MODEL-MANAGEMENT-PLAN.md` 10.8 第 6/10 条。）

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
