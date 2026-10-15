# 模型能力 / 思考档位 / 厂商分组 — 规划文档

> 状态：**v6（阶段 A / B / C / D 全部完成并各自通过独立审查，2026-10-14）**。
> v5 关键补充：阶段 B 的匹配语义已用真实数据端到端仿真确认——ZCode 是**全串锚定 `^(?:...)$` + 大小写不敏感**、**逐字段覆盖**（不是取首条命中）、`deepseek-v4.1-flash` 内建即有条目（详见 4.0 / 4.0b）。阶段 C 的「自适应档位 + 高级自填开关」已完成，并补修了 DSH 读/写不对称（见 5.1 / 5.2）。阶段 D 的厂商分组视图已完成（见 6.1）。
> 历史更正：A-7 的修法被实测推翻——`config.enabled` 是「模型是否在列表中启用」，**不是**思考开关（详见 A-7 语义节）。

> 目标已与用户确认；执行顺序：**阶段 A（修缺陷止血）→ B（能力表地基）→ C（档位自适应）→ D（厂商分组）**。
> 依据：APM 源码与测试 + ZCode 安装目录的应用自带资源（`zcode-builtin.json` / `app.asar`）+ DSH 安装目录的 `app.asar`。**全程未读取任何用户配置**。

## 修订摘要（v1 → v2）

| 项 | v1 的说法 | 核实后 |
|---|---|---|
| 内建规则数量 | 484 条 `modelMatch` 正则 | 484 **条规则**，其中只有 **210 条**含 `modelMatch` 正则；其余 274 条按 `templateId`/`modelId` 定向 |
| off 名 | 「多数 disabled，gpt-5.4/5.6、qwen3.8-omni 是 none」 | 方向正确但不完整：**还有若干家族根本没有 off（不可关）** |
| `["enabled"]` 的模型 | 「kimi-k2.6 / kimi-k2.7-code / glm-5.1 只有 enabled」 | 仅站点层成立。通用 `modelRules` 里 kimi-k2.6、glm-5.1 是 `["disabled","enabled"]`（可关），只有 kimi-k2.7-code 是 `["enabled"]` |
| `resolveLegacyReasoningLevel` | 「不在 values 里就回落 disabled」 | 机制误读，实为「按该家族**旧名**（`off`/`nothink`）重写后命中才返回 `disabled`，其余未知值返回 `undefined`」 |
| DSH 档位缺口 | 「无入口 / minimal 降级」 | 比这严重：**读写两端都丢弃** `medium`/`xhigh`（读端还丢 `enabled`），且被既有测试锁定 |
| 「删 `OpencodeEffortToVariant` 的 minimal→low」 | 净所必行 | 该 map 已是**死代码**，改它无效；真正的 minimal→low 在别处 |
| 缺陷清单 | A-1 … A-6 | 新增 **A-7（ZCode 关闭思考完全不落盘）**、**A-8（单 off 集合丢档位）**，A-7 比 A-2 更严重 |
| 写入路径 | 只列 `CardsToCfgMap` 一条 | 还有第二条不经卡片语义的路径（旧版导入） |
| 数据源 | ZCode 正则 + 自维护覆盖层 | 增列 **DSH 内嵌 pi-ai JSON 目录**为第二数据源（精确模型 id） |
| 前端硬编码档位表 | 2 处 | **3 处**（另有导入页）+ 3 处 `off/high/max` 兜底 |
| 新增设置项落地点 | 4 处 | **6 处**（漏了后端 `GetOptions` 聚合与前端启动回读） |
| 既有关联开关 | 未提及 | 已有 `reasoningAllIntensities` 与 C2 语义重叠，必须先决定复用还是替代 |
| ZCode 版本 | 3.15.1 | 本机为 **3.15.3**（内建数据 `schemaVersion=1, revision=31`）；早期笔记的 3.15.1 不影响结论 |

## 0. 已确认的目标

| # | 目标 | 结论 |
|---|---|---|
| 1 | 自动配置补全 | 先打地基；数据**两路并行**：借用客户端内建数据 + APM 自维护一份 |
| 2 | 思考强度分级 | 固定档位全集对**所有**模型可选，但按模型**自适应**启用其适合的档位；其余档位允许用户手动启用；另提供**高级开关**让用户像 ZCode 一样自填档位 |
| 3 | 厂商自动分类 | **只做 APM 内部视图**（分组 / 筛选 / 整组批量开关），不写入任何配置文件 |
| 4 | 顺序 | 先修 #2 缺陷止血 → #1 地基 → #3 |

## 1. 现状事实

### 1.1 自动配置补全

已有：
- `internal/core/presets.go`：手维护的 `model ID → {Context, Output}` 表（**实测 212 条**），`LookupModelLimits` 走「精确 ID → 最长前缀」匹配（`presets.go:273-292`）。
- `BuildModelCards(models, autoFill)`：仅在 API 未返回对应字段时用预设兜底（`cards.go:22-35`）。
- 设置项 `auto_fill_limits`（默认开，`app.go:760-777/815-821`）；手动覆盖入口 `ApplyModelPresets`（`app.go:1434`，前端 `ManagePage.vue:625-648`）。
- `GuessReasoning(modelID)`：关键词表推断「是否支持思考」，只有布尔（`constants.go:43`、`infer.go:17`）。

缺口：
- 只有 context/output 两列，没有档位集合、没有模态、没有厂商。
- 前缀匹配会漏。**已实测确认** `deepseek-v4.1-flash`、`deepseek-v4.1-pro`、`qwen3.8-omni-flash` 都匹配不到任何预设（表内没有任何键是它们的前缀，`presets.go:78-85`）。这几个恰好就是缺陷 A-1（off 名被改写）涉及的家族。

### 1.2 思考档位的真实形态

**ZCode（配置格式）**
- `modelRule.config.optionSpecs.reasoningLevel = {values: string[], map: <表达式字符串>}`（`zcodeprovider/schema.go:199-207`）。zod 约束：`values` 是非空数组、去重、每项 trim 后非空；`map` 是可编译的表达式字符串。
- **`values` 不是全局枚举**，每个模型各写各的。权威数据在 `C:\AppData\Zcode\resources\config\provider\zcode-builtin.json` 的 `config.modelConfigRules`。
- 结构：`modelRules`(84) + `modelApiRules`(72) + `providerSiteRules`(54) + `templateModelRules`(244) + `builtinProviderModelRules`(30) = **484 条**；**其中 210 条带 `modelMatch` 正则**，274 条按 `templateId`/`modelId` 定向。
- **`values` 写在 `modelRules`（模型层）；`map` 主要写在 `modelApiRules`（按 `apiTypeMatch`，71/72）与 `providerSiteRules`（按 `baseUrlMatch`，35/54）**；ZCode 用 `resolve({providerId, modelId, templateId, apiType, baseUrl})` 把各层合并成最终 `optionSpecs.reasoningLevel`。`map:"{}"` 合法，表示本层不追加请求字段。
- 词表实测恰好 **9 个**：`none / disabled / enabled / minimal / low / medium / high / xhigh / max`（无 `off`/`nothink`，那是旧版 `config.json` 拼法；无第 10 个词）。
- **off 的名字随家族变，而且部分家族根本没有 off（不可关）**：
  - `disabled`：多数家族，例如 `deepseek-v4-flash` / `deepseek-v4.1-flash` = `["disabled","low","high","max"]`。
  - `none`：`gpt-5.4` = `["none","low","medium","high","xhigh"]`，`gpt-5.6` = `["none",…,"max"]`，`qwen3.8-omni-flash` = `["none","minimal",…,"max"]`。
  - `["enabled"]` 单元素：通用层只有 `kimi-k2.7-code`；`kimi-k2.6`、`glm-5.1` 在通用层是 `["disabled","enabled"]`（可关），只在 opencode-zen 站点层才收敛为 `["enabled"]`。
  - **完全无 off**：`glm-5.3` = `["low","high","max"]`，以及站点层的 `glm-5.2`、`deepseek-v4-pro`、`kimi-k3`、`qwen3.8-max|flash`、`gpt-5.3-codex`、`gpt-5.4-pro`、`grok-4.6`、`claude-5*`、`gpt-6-astra` 等。
- `resolveLegacyReasoningLevel`（app.asar，符号 `LF`）：把该家族 values 里的 `disabled` 按旧名（`off`/`nothink`）重写后，若持久化值命中才返回 `"disabled"`，否则返回 `undefined`。**另有第二处渲染进程实现（取 `values.at(-1)`）**，所以「未知值回落 disabled」只是推断，不要当成已证实的前提。

**DSH（DeepSeek Harness）**
- 硬枚举 `THINKING_LEVELS = ["off","minimal","low","medium","high","xhigh","max"]`（7 个，此顺序）。`thinkingBudgets` 键 = `minimal/low/medium/high`。
- `reasoningEfforts = z.union([z.const(false), z.dict(z.union([z.string(), z.const(null)]), z.union(THINKING_LEVELS))])`：键必须是上述档位，值是协议拼法；省略=沿用内建目录能力，`false`=非推理模型，空 dict 报错，`null` 只允许 `off`，且必须声明至少一个非 off 档位。不支持思考的模型 → `["off"]`。
- **存在一份可脚本抽取的内建模型目录**：pi-ai 的 JSON 目录嵌在 `C:\AppData\DSH Desktop\resources\app.asar` 内（形如 `{"id","name","api","provider","baseUrl","reasoning","contextWindow","maxTokens","thinkingLevelMap",...}`，以**精确模型 id** 为键，约 30 个来源文件、数百条条目）。`thinkingLevelMap` 用的正是 DSH 的 7 档键。这是阶段 B 的第二数据源。

### 1.3 APM 的三条写入路径与卡片语义

| 路径 | 入口 | 档位处理 |
|---|---|---|
| **卡片路径（主）** | `本页保存/导入`：cards → `BuildProviderCfgEditor` → `CardsToCfgMap`(`cards.go:319`) → `CardToCfg`(`cards.go:182-317`) → `zcodeprovider.FromLegacy` → `modelRuleFromLegacy`(`convert.go:185-225`) → `legacyReasoningVariants`(`convert.go:232-255`) | `off → disabled`；`none`/`nothink`/`minimal` 在此层原样透传（但已在更早的 `NormalizeVariants` 里被折成 `off`/`low`） |
| **旧版导入路径** | `zcode_migrate.go:106-117`（UI 可达，`app.go:1322`） | 直接用 `FromLegacy` 把旧 `reasoning.variants` 写进 `values`，**不经 cards / `NormalizeVariants`**：只做 `off → disabled`，`none`/`nothink`/`minimal` 原样落盘 |
| **DSH 路径** | `deepseekLegacyStore`(`backend_deepseek.go:111-119`) 与 `deepseekProfileStore`(`backend_deepseek_profile.go:288-320`) | 两者档位来源一致（都经 `BuildProviderCfgEditor` + `dshApplyExplicitReasoningOff` + `deepSeekBuildEntry` → `dshModelToRaw`）。home 级 `cordis.patch.yml` 不是第三个写后端，而是「被覆盖即拒写」 |

读入链（ZCode）：`v2ModelCard`(`backend_zcode_v2.go:315-355`) 把 `disabled → off`，其余原样，**从不写 `defaultVariant`** → `CfgToCard`(`cards.go:76-180`) → `NormalizeVariants` → `canonicalVariant`(`tokens.go:73-82`)：`none/disabled/nothink → off`、`minimal → low`。

写盘语义（关键）：`zcodeV2Store.Upsert` 先 `removeV2ModelRules` 清掉该 provider **全部**模型规则，再按本次提交逐条重建（`backend_zcode_v2.go:226-245`）——**每次保存都会重写该 provider 全部模型的 `reasoningLevel.values`**；提交里没有的模型会消失。保留的只有模型级 `config.enabled` 与 `optionSpecs.{reasoningLevel,maxOutputTokens}.map`（`:403-430`），以及 providerRule 级的 `providerId/providerName/templateId/enabled` + `config.logo/builtinModelIds/personalModelIds/modelOrder/visibility` + `api.headers` + `access` 未被 incoming 提到的键（`:359-399`）。

DSH 读写（更严重的不对称）：
- 读 `ConvertDeepSeekModel`(`deepseek.go:201-299`)：只保留 `off/low/high/max`，**丢弃 `medium`/`xhigh`，并把 `enabled` 也丢掉**；`minimal → low`（`:212-213`）。
- 写 `dshModelToRaw`(`deepseek.go:574-661`)：`reasoningEfforts` 组装在 `:609-648`，**独立白名单 `:619` 只放行 `off/low/high/max`**，默认 `{"off":nil,"high":"high","max":"max"}`(`:629`)。
- 该行为被既有测试 `dsh_regression_test.go:178-224` 显式锁定。

## 2. 缺陷清单（阶段 A 的输入）

| ID | 缺陷 | 证据 | 影响 | 触发条件 |
|---|---|---|---|---|
| **A-1** | `none` 被改写成 `disabled` | `canonicalVariant` `none→off`(`tokens.go:75`/`:101`) + `legacyReasoningVariants` `off→disabled`(`convert.go:245`) | 与模型 `map` 表达式里写死的 off 名不一致 → 关闭思考失效或发出错误值 | values 含 `none`（gpt-5.4/5.6、qwen3.8-omni 家族） |
| **A-2** | values 非空且不含 `high` 时**保存报错** | `v2ModelCard` 不写 `defaultVariant`；`CfgToCard` 取不到→`""`(`cards.go:104-109`)；`CardToCfg` 回落 `DefaultVariantFor(kind)="high"` 后报错(`cards.go:238-244`) | 打开该提供商再保存直接失败 | 任一条 `reasoningLevel.values` 非空且不含 `"high"`：`["enabled"]`、`["low","medium"]`、`["max"]` 均中招；`config.enabled` 缺省或 `true` 都触发 |
| **A-7** | **ZCode 侧「关闭思考」语义错位**（比 A-2 严重） | `CardToCfg` 只在 `card.Reasoning==true` 时写 reasoning 块(`cards.go:233`) | 关闭思考后保存 = 删掉整个 reasoning 块，`values` 整段消失 | 非空 values + 用户关闭思考。**修法已更正，见下方「A-7 语义（v3 已证实）」** |
| **A-8** | 单元素 off 集合静默丢档位（连 `map` 一起丢） | 读回 `Reasoning=false` → 不写 reasoning 块；`mergeV2ModelRuleConfig` 在 incoming 无 `optionSpecs` 时 `continue`(`:409-429`) | `["disabled"]`/`["none"]` 这类集合的档位与映射丢失 | values 只有单个 off 词 |
| **A-3** | 固定档位 UI 表达不了真实集合 | `ModelCard.vue:101`、`ManagePage.vue:263`、**`ImportPage.vue:200`** | `["enabled"]`、`["low","high","max"]` 的档位在胶囊中不显示，默认档下拉也选不出 | 集合不等于 UI 的固定 6 档 |
| **A-4** | ZCode 侧「默认档位」是无效字段 | v2 modelRule.config 只有 `enabled/properties/optionSpecs`(`schema.go:170-174`)；`modelRuleFromLegacy` 不读 `defaultVariant`；`mergeV2ModelRuleConfig` 不恢复它 | 用户以为设置了默认档位，实际不落盘（仅 DSH 有效，见 `deepseek.go:632-643`） | 恒成立 |
| **A-5** | 新建/开启思考时默认写 `["disabled","high","max"]` | **前端为主**：`ModelCard.vue:109-112`（`reasoningAllIntensities` 开则写全量）+ `ManagePage.vue:595` + `ImportPage.vue:344`；后端 `DefaultVariantsFor`(`infer.go:73-82`) 只是兜底，二者结果恰好相同 | 对无 off 的家族（如 `glm-5.3`）会发出 `effort="disabled"` | 用户开启思考且未手改档位 |
| **A-6** | DSH 档位被硬限制为 4 档且往返丢数据 | 读 `deepseek.go:225-231`（丢 `medium`/`xhigh`/`enabled`）、写 `:619`/`:629`、既有测试 `dsh_regression_test.go:178-224` | 往返已有 7 档配置会丢 `medium`/`xhigh`，`minimal` 降为 `low` | 恒成立 |

### A-7 语义（v3 已证实，推翻了 v2 的修法）

实测内建数据（`zcode-builtin.json` 只读统计）：`config.enabled` 全仓 **275 处**，分布为 `templateModelRules` 74 true / 170 false + `builtinProviderModelRules` 30/30 true，**`modelRules` / `modelApiRules` / `providerSiteRules` 三张模型能力表里一处都没有**。

结论：`config.enabled` 是**模板/内建列表里「该模型是否默认启用」**（例如 `glm-5.2` 在 `openrouter` 模板下 `false`，因为它不在此站点模型列表中；同一模型在 `opencode-zen-chat` 下是 `true`），**与思考开关无关**。

**ZCode 里「关闭思考」的真实表达 = 把档位集合收敛到 off 档位**：`modelApiRules` 的 `map` 表达式直接证明了这一点——
`reasoningLevel == "disabled" ? {thinking:{type:"disabled"}} : {thinking:{type:"enabled"}, ...}`。
即 off 档位（`disabled`/`none`）本身就是 `values` 里的一个普通词，选中它就等于关闭思考。

因此：
- **绝不能**用 `config.enabled=false` 表达关闭思考（会误关模型在列表中的显示）。
- A-7 的真实缺陷是：卡片 `Reasoning=false` 时 `CardToCfg` 整个删掉 `reasoning` 块 → `values` 丢失 → 用户下次看不到档位。
- **正确修法**：`Reasoning=false` 时**保留** `values`（原集合）与 `map`，只把「当前生效档位」表达为 off。由于 v2 modelRule 没有「当前档位」字段（那是 `map` 里运行时求值的 `reasoningLevel`，配置里不存在），所以 APM 侧的最小正确行为是：**关闭思考 = 保留完整 `values`（含 off 词）不动**，让用户在 ZCode 里选。若用户就是想「没有档位可选」，则退化为只写 off 档位单元素集合（A-8 保留语义）。

**旁路风险（不列为缺陷，但要在文档里留痕）**
：`DeepSeekSaveProvider*`(`deepseek.go:541-572`) 直接接受已构建的 `providerCfg`（可带裸 `reasoningEfforts`），不经卡片语义与 `dshApplyExplicitReasoningOff`，当前仅测试引用但属导出 API；`core.OpenCodeSaveProvider`(`opencode.go:261-273`) 无生产调用方。旧 OpenCode 分支仍在 `app.go:1021-1057`（`opencode.go:159-187`，`off→none` 是**第五种**档位拼法），Agent 已从 `AllAgents` 隐藏(`agents.go:28-35`)但分支与导入保留。

## 3. 阶段 A — 修复档位缺陷（先止血，不依赖新数据）

### 执行状态（阶段 A 已完成 + 验证修复轮 2026-10-14）

| 任务 | 负责 | 状态 |
|---|---|---|
| A1 档位别名往返保真（A-1、A-8） | Sub Agent A | 完成 |
| A2 默认档位回落 + 关闭思考语义（A-2、A-7） | Sub Agent A | 完成 |
| A4 DSH 七档放开（A-6） | Sub Agent B | 完成 |
| A3 ZCode 默认档位无效（A-4） | 主会话 | 完成（卡片加「仅对 DSH 生效」提示） |
| 前端档位表收敛 + `_off_alias` 透传（A-3、A-5） | Sub Agent C | 完成 |
| **独立验证轮**（需求符合性 / 代码审查 / 改进点，三个只读 Agent） | 主会话编排 | 完成 |
| **S1 关闭思考持久化回退**（验证轮发现，本改动引入） | Sub Agent D + E | 完成 |
| **M1 默认档位策略统一 high 优先** | Sub Agent D | 完成 |
| **M2 legacy 写入剥离内部键 `offAlias`** | Sub Agent D | 完成 |
| **M3 `enabled` 档位前端可见可编辑** | Sub Agent E | 完成 |
| **M6 新文案入 i18n 三语** | Sub Agent E | 完成 |
| **死代码清理**（`OpencodeEffortOrder`、`OpencodeEffortToVariant`） | Sub Agent D | 完成 |
| 集中验证（修复轮） | 主会话 | 完成 |

#### 验证轮发现的阻断项 S1 与其修法

**S1（本改动引入的功能回退）**：阶段 A 为「关闭思考时保留档位集合」牺牲了关闭状态的可持久化——关闭后 `variants` 仍含非 off 档，写回配置重读时 `Reasoning` 被反推为 true，开关翻回开启。

**根因有两重**（第二重是 Sub Agent D 修复时挖出的）：
1. `CardToCfg` 在 `!Reasoning && len(variants)>0` 时仍写完整集合；
2. `v2ModelCard` 把 v2 `config.enabled` 映射进 `reasoning["enabled"]`，使 `CfgToCard` 采信该布尔、**跳过**由 `values` 反推 `Reasoning`——即便收敛出 `["disabled"]`，带 `config.enabled` 的模型读回仍是 `Reasoning=true`。

**修法（客观依据，非取舍）**：ZCode 的「关闭思考」= 档位集合收敛为单元素 off 词（`modelApiRules` 的 map 表达式 `reasoningLevel == "disabled" ? {thinking:{type:"disabled"}} : {thinking:{type:"enabled"},...}` 证明无独立思考开关字段）。
- 后端：`CardToCfg` 关闭时把 `variants` 收敛为 `["off"]`；**删除** `v2ModelCard` 的 `config.enabled → reasoning.enabled` 映射（`config.enabled` 仍由 `mergeV2ModelRuleConfig` 原样保留，不丢数据）。
- 前端：`toggleReasoning` 关闭时置 `variants = ['off']`；**档位集合不含 off 词的模型**（如 `glm-5.3` = `["low","high","max"]`、`kimi-k2.7-code` = `["enabled"]`）思考开关置 `disabled` 并提示「此模型的档位集合不含关闭档，无法关闭思考」；批量操作跳过这类卡片并 toast 计数提示。

#### 验证轮修复后的集中验证结果

`go build ./...`、`go vet ./...`、`go test ./... -count=1` 全通过；前端 `npm run build`（含 `vue-tsc --noEmit`）通过；`git diff --check` 干净。关键新增测试：`TestS1CloseThinkingPersistsAcrossV2RoundTrip`（端到端断言关闭后重读 `Reasoning=false`、`Variants=["off"]`、`OffAlias="disabled"`）、`TestA8SingleOffRoundTripPreserved`、`TestLegacyWriteStripsOffAlias`、`TestStripAPMInternalModelKeysRemovesUnderscoreAndOffAlias`、`TestPickDefaultVariantHighFirst`。

#### 需求符合性的诚实结论（独立验证）

三条需求里，**#2 只完成了「修缺陷」部分**（止血做了，用户澄清的「按模型自适应启用档位 + 高级开关自填」形态未做，依赖 #1 能力表）；**#1 与 #3 尚未开始**。按「先 #2 → #1 → #3」顺序，只走完第一步的修复阶段。用户点名的 `deepseek-v4.1-flash` 等模型在旧 `presets.go` 前缀匹配下**仍匹配不到**，需阶段 B 的两路数据源才能解决。

**A1. 档位别名往返保真（覆盖 A-1、A-8）**
- `ModelCard` 增加内部字段（下划线前缀，遵循 `models.go` 约定）：`_off_alias`（配置里 off 的原始拼法）与 `_variant_aliases`（档位 → 原拼法映射，覆盖 `minimal` 等被折叠过的值）。
- 读入侧：`v2ModelCard`、`CfgToCard` 记录别名。
- 写出侧：`legacyReasoningVariants` 按别名回填；缺失时按目标格式兜底（ZCode → `disabled`，DSH → `off`）。
- 边界：ZCode 侧 `values` 必须落在该模型集合内（据 `resolveLegacyReasoningLevel` 推断，**属推断需实测**）；DSH 侧还要过 `dshModelToRaw` 的白名单。

**A2. 默认档位回落与「关闭思考」语义（覆盖 A-2、A-7、A-8）**
- 默认档位规则改为「集合内首个非 off 档位优先（按 `DefaultVariantPreference` 顺序），否则 off 档位」，`DefaultVariantByKind` 仅作最后兜底且必须落在集合内 → 彻底消除 A-2 的保存报错。
- **关闭思考（A-7）**：绝不动 `config.enabled`。卡片 `Reasoning=false` 时，**保留原 `values` 与 `map`**（`CardToCfg` 仍写 reasoning 块，或走 `mergeV2ModelRuleConfig` 保留 prev 的 optionSpecs）；仅当原集合只剩单元素、用户明确要「取消档位」时才收敛。这样既保住档位又不再删数据。

**A3. ZCode 侧处理「默认档位」无效（覆盖 A-4）**
- 隐藏该控件或标注「仅 DSH 生效」。

**A4. DSH 档位补齐（覆盖 A-6）— 必须同时改三处 + 改测试**
1. `canonicalVariant`(`tokens.go:73-82`) 去掉 `minimal→low`；
2. 读 `ConvertDeepSeekModel`(`deepseek.go:201-299`) 不再丢 `medium`/`xhigh`/`enabled`；
3. 写 `dshModelToRaw`：白名单(`:619`)与默认值(`:629`)放开到 7 档；
4. 更新 `dsh_regression_test.go:178-224` 的既有断言。
- **注意**：`OpencodeEffortToVariant`/`OpencodeEffortOrder`(`constants.go:80-87`) 已是**死代码**（查表前输入已被 `canonicalVariant` 归一，键恒 miss），改它无效；它可删。

**A5. 回归测试（表驱动，`t.TempDir()` 沙箱，只用占位值）**

v2 集合往返（断言 `values` 与 off 拼法不变、`map` 不被动）：
1. `["enabled"]` — A-2
2. `["disabled","enabled"]` — A-2（当前也会报错）
3. `["none","low","medium","high","xhigh","max"]` — A-1
4. `["low","medium","xhigh","max"]` — 新增，证明 A-2 与 `enabled` 无关
5. `["max"]` — 新增，单元素、无 high、无 off
6. `["disabled"]` — 新增，A-8，断言**不丢**
7. `config.enabled:false` + 非空 values — 新增，A-7
8. `_raw_reasoning_enabled=false` + 非空 values 直接过 `CardToCfg` — 新增，暴露 ZCode 无对应实现
9. `mergeV2ModelRuleConfig` 在「保留该 optionSpec」与「整个 optionSpecs 被删」两种情形下的去留
10. 旧版导入路径（`ApplyLegacyImport`）写入 `values` 的别名保真

DSH：
11. 7 档 `reasoningEfforts` 往返 + `dshApplyExplicitReasoningOff`（`reasoningEfforts:false`、清单数 `reasoningEffort`）联合断言

**验收**：上述测试通过；`go test ./...`、`go vet ./...`；在 Wails 窗口人工验证这几种集合的编辑与保存。

## 4. 阶段 B — 模型能力表地基（#1）

### 4.0 主会话实测证据（2026-10-14，决定设计的三条硬事实）

1. **210 条 `modelMatch` 全部通过 Go RE2 编译（0 失败）**。实测方式：临时 module 里逐条 `regexp.Compile`，`total=210 bad=0`。内建模式只用到 `.*`、`(?:...)`、字符类和 `[.\-:/\[].*` 收尾，无回溯断言、无反向引用——**Go 原生 `regexp` 可直接用，不需要引入 PCRE 依赖**。
1b. **匹配是全串锚定 + 大小写不敏感**（v4 补充，两条都是踩坑后实测的）。ZCode 内部分配器用 `new RegExp("^(?:" + modelMatch + ")$")`，所以生成的 `Re` 必须是 `(?i)^(?:PATTERN)$`：漏掉锚定会让 `gpt-5.6-luna-2026-01-01`、`glm-5.3-flash` 各多命中一条；漏掉 `(?i)` 会让 `glm-5.2`（provider 返回的小写形态）退到 `.*glm-5` 的 ctx=200000，而正确值是 1000000（来自大写的 `.*GLM-5\.2` 规则）。84 条里有 **10 条含大写字母**（`GLM-5.2`、`GLM-5-Turbo`、`MiniMax-M3/M2/M2.7/M2.5/M2.1` 及其 highspeed 变体），全部踩中小写模型名。→ 生成器统一写 `(?i)^(?:...)$`，匹配器直接用 `rule.Re.MatchString`。
2. **规则是「有序 + 逐字段覆盖」**，不是「交集」。`modelRules` 首条恒为 `.*`（ctx=200000/maxOut=32000/`["disabled","enabled"]`）作为兜底，其后按家族由泛到细排列；后命中的规则里**出现过的字段覆盖前面的值，未出现的字段保持前面的值**。证据：`modelRules[6]` `glm-5\.3` 给 ctx=1000000/rv=`["low","high","max"]`，`modelRules[7]` `glm-5\.3-flash` 只给 `inputFormat`（无 ctx、无 optionSpecs）——合并后 `glm-5.3-flash` 必须继承 `glm-5.3` 的 ctx 与档位，仅补上多模态开关。**这条决定了 B3 必须是「全表顺序扫描 + 逐字段覆盖」，不能取「第一条命中」。**
3. **`deepseek-v4.1-flash` 已经被内建数据覆盖**：`modelRules[64]` = `.*deepseek-v4[.-]1-flash(?:[.\-:/\[].*)?` → ctx=1000000 / maxOut=384000 / rv=`["disabled","low","high","max"]`，正是用户举例想要的值。它今天填不出来，**纯粹因为 `presets.go` 只有精确/最长前缀匹配**：键 `deepseek-v4-flash` 不是 `deepseek-v4.1-flash` 的前缀（中间多了 `.1`），前缀匹配在 `1` 处断裂。→ 走正则路一即可解决 #1 的原始诉求。

### 4.0b 逐字段覆盖的仿真结果（照抄进单测）

用真实 84 条 `modelRules`、`(?i)^(?:...)$` 锚定、按序逐字段覆盖，实测结果（`hits` = 命中的规则条数）：

| 模型 ID | hits | Context | Output | 规范化档位 |
|---|---|---|---|---|
| `deepseek-v4.1-flash` | 2 | 1000000 | 384000 | `off,low,high,max` |
| `deepseek-v4-flash` | 2 | 1000000 | 384000 | `off,low,high,max` |
| `deepseek-flash` | 2 | 1000000 | 384000 | `off,low,high,max` |
| `glm-5.3-flash` | 4 | 1000000 | 128000 | `low,high,max`（**无 off**） |
| `glm-5.3` | 3 | 1000000 | 128000 | `low,high,max`（**无 off**） |
| `glm-4.6` | 2 | 200000 | 131072 | `off,enabled` |
| `glm-5.2` | 3 | **1000000** | 128000 | `off,high,max` |
| `kimi-k2.7-code` | 2 | 262144 | 98304 | `enabled` |
| `kimi-k2.6` | 2 | 262144 | 98304 | `off,enabled` |
| `gpt-5.6-luna-2026-01-01` | 2 | 1050000 | 128000 | `off,low,medium,high,xhigh,max` |
| `qwen3-max` | 2 | 262144 | 65536 | `off,enabled` |
| `my-local-model`（未知） | 1 | 200000 | 32000 | `off,enabled` |

要点：**`glm-5.3-flash` 的档位不含 off**（家族规则整体替换了兜底的 off），这正好是阶段 A 的 `canToggleReasoning` 不可关场景；**`glm-5.2` 的 1000000 只有大小写不敏感才拿得到**。这两条是单测里最有价值的断言。

**但上表最后一行的行为被 APM 主动否决**：`my-local-model` 只命中第 0 条 `.*` 兜底（实测确认全局仅此一条 Pattern 恰为 `.*`），若采纳就会把**任意未知模型**填成 ctx=200000/out=32000。APM 对未知模型的既有契约是「留空自动匹配」——写 200000 进一个真实只有 32k 的本地模型配置会把它弄坏。→ **实现里显式跳过 `Pattern == ".*"` 这一条规则**（有意偏离 ZCode 的兜底默认值），未知模型因此三字段皆空，仍由 `presets.go` 旧表兜底。这是本阶段唯一一处「不忠实照搬客户端行为」的决定，理由与位置写进代码注释。

### 4.0c 跳过兜底后的权威结果（v4 终稿，生成文件已编译实测）

把生成文件放进临时 module、加一份契约副本真正编译执行，得到下表（跳过 `.*`、`(?i)^(?:...)$`、逐字段覆盖、规范化）：

| 模型 | ctx | out | 规范化档位 |
|---|---|---|---|
| `deepseek-v4.1-flash` | 1000000 | 384000 | `off,low,high,max` |
| `deepseek-v4-flash` | 1000000 | 384000 | `off,low,high,max` |
| `deepseek-flash` | 1000000 | 384000 | `off,low,high,max` |
| `glm-5.3-flash` | 1000000 | 128000 | `low,high,max` |
| `glm-5.3` | 1000000 | 128000 | `low,high,max` |
| `glm-5.2` | 1000000 | 128000 | `off,high,max` |
| `glm-4.6` | 200000 | 131072 | `off,enabled` |
| `kimi-k2.7-code` | 262144 | 98304 | `enabled` |
| `kimi-k2.6` | 262144 | 98304 | `off,enabled` |
| `gpt-5.6-luna-2026-01-01` | 1050000 | 128000 | `off,low,medium,high,xhigh,max` |
| `qwen3-max` | 262144 | 65536 | **空**（nil） |
| `minimax-m2.5` | 204800 | 32000 | **空**（nil） |
| `my-local-model`（未知） | 0 | 0 | 空 |
| `deepseek-chat` | 0 | 0 | 空 |

**重要副作用**：跳过兜底不只是清掉未知模型的猜测值，还会让**只有兜底给过档位的模型**（`qwen3-max`、`minimax-m2.5`）档位变空——它们自己的规则没带 `reasoningLevel.values`。这是正确行为：APM 不该为这些模型编造档位集合，阶段 C 的自适应逻辑会把「档位为空」当作「未推断到，展示全集但不高亮」。**空集合一律置 nil**（不是 `[]`），以配合 `models.go` 的 `omitempty`。

补充统计：三张表共 210 条带 `modelMatch` 的规则（`modelRules` 84 / `modelApiRules` 72 / `providerSiteRules` 54）；`config.enabled` 仅出现 1 次；无 `null` 字面值（不存在「显式清空」语义，可安全做逐字段覆盖）。`modelApiRules` 通过 `apiTypeMatch` 区分协议，其 `reasoningLevel` **只有 `map` 没有 `values`**（值集来自 `modelRules`/`providerSiteRules`）；`providerSiteRules` 额外带 `baseUrlMatch`（命中需模型名 + 协议 + baseURL 三者同时匹配）。

### 4.1 数据模型（B1）

新增 `internal/core/capability.go`，能力条目：

```go
type ModelCapability struct {
    Context         int      // 0 = 未知
    Output          int      // 0 = 未知
    ReasoningLevels []string // 规范化后的 off 档 + 强度档；nil = 未知（不推断）
    Vendor          string   // 家族归属，未命中 "unknown"
    Source          string   // "provider" | "override" | "zcode" | "site" | "catalog" | "preset"；仅供测试与调试，本身不进 DTO
}
func LookupModelCapability(modelID string) (ModelCapability, bool)
```

保留 `LookupModelLimits` / `ApplyModelPresets` 的对外签名与行为（内部改为委托能力表），避免一次性改动 3 个消费点。

### 4.1b 执行状态（阶段 B 已完成 2026-10-14）

| 交付物 | 状态 | 说明 |
|---|---|---|
| `scripts/sync-model-catalog.mjs` | ✅ | 只抽 `modelRules` 84 条；`--check` 幂等；头部声明来源/用途/不写回 |
| `internal/core/model_catalog_gen.go` | ✅ | 84 条，源 revision 31，`Re: (?i)^(?:...)$` 预编译，只声明一个 var |
| `internal/core/capability.go` | ✅ | `catalogRule` 类型、`ModelCapability`、`LookupModelCapability`、`mergeCatalog`、`lookupCapability`、`detectVendor` |
| `internal/core/model_overrides.go` | ✅ | 覆盖层（初始 1 条 `deepseek-v4.1-flash`），精确 + 最长前缀 |
| `internal/core/cards.go` / `infer.go` / `models.go` | ✅ | 卡片接入 + `GuessReasoning` 改走能力表 + `_vendor`/`_supported_levels` 内部字段 |
| `frontend/src/types/index.ts` | ✅ | 镜像 `_vendor` / `_supported_levels` |
| `internal/core/capability_test.go` | ✅ | 14 条用例 |

**集中验证（主会话全部命令）**：`go build ./...` OK、`go vet ./...` OK、`go test ./... -count=1` 全绿（6 个包）、`npm run build`（vue-tsc + vite）OK、`node scripts/sync-model-catalog.mjs --check` 退出 0、`node scripts/sync-version.mjs --check` 一致、`go mod verify` OK、`git diff --check` 干净。

**过程修复 1 — 测试构造 bug**：`TestMergeCatalogFieldWiseOverwrite` 的合成模式 `.*model-x` 被锚定后要求以 `model-x` 结尾，匹配不到 `model-x-narrow`；改为 `model-x.*` 后通过。

### 4.1c 独立代码审查轮（2026-10-14，只读 Agent）与修复

一个只读审查 Agent 独立复核了全部阶段 B 代码（含在仓库副本里跑探针，未改动真实仓库），发现 3 个真实缺陷，**均由主会话亲自复核成立后修复**：

- **B1（阻断）**：`stripAPMInternalModelKeys` 在唯一生产调用点（`zcodeLegacyStore.Upsert`）是**空操作**。函数只找顶层 `providerCfg["models"]`，而 `MergeProviderIntoConfig` 返回的 `merged` 形状是整份文档 `{provider:{"<id>":{models:{...}}}}`，循环体从不执行 → `reasoning.offAlias` 照样落进用户 legacy `config.json`。**即阶段 A 声称修好的 M2 实际未生效。**
  **修法**：剥离函数改为支持两种形状（单 provider 的 `{models:...}` 与整份文档 `{provider:{...:{models:...}}}`），抽出 `stripInternalModelKeysInModels` 复用；顺带把冗余的显式键列表收敛为 `strings.HasPrefix(key, "_")`。
  **验证**：新增 `TestStripAPMInternalModelKeysWholeDoc`（以整份文档为输入）+ 变异测试——把整份文档分支临时改回旧行为后，`TestLegacyWriteStripsOffAlias` 与新用例**如期变红**，还原后变绿，证明测试真能捕获该缺陷。
  **影响面闭合**：ZCode 两种格式都注册了 backend（legacy 走 `RegisterBackendFor`、v2 走 `RegisterBackendForFormat`），legacy 写盘必经 `Upsert`；app.go 里两条直调 `MergeProviderIntoConfig` 的兜底写盘路径只服务「无注册 backend 的 agent」，而当前 Agent 集合仅 zcode/deepseek，二者均有 backend → **这两条路径不可达**，无残留泄漏。
- **M1（应修）**：守护 B1 的测试 `findOffAliasInDoc` 用 `case map[string]interface{}` 递归，但 `merged` 的静态类型是命名类型 `Doc`（`type Doc map[string]interface{}`），Go 类型 switch 按**精确动态类型**匹配，`Doc` 不匹配 → 在根节点直接返回空串，断言恒真（`TestLegacyWriteStripsOffAlias` 在 B1 存在时依然 PASS）。**修法**：改用 `reflect` 递归 `findKeyString`，兼容任意 map 类型；并新增 `TestStripAPMInternalModelKeysWholeDoc` 锁住真实调用形状。
- **M2（应修）**：生成脚本 `--check` **不幂等**——生成物头部含 `抽取日期: <today>`，而 `--check` 做整文件字节比较，跨天后即便内置资源一条未变也会误报失败。**修法**：文件头去掉日期，只保留「源 revision + 规则条数」（只随真实数据变化）；顺带修 S4（头部原写死 `DEFAULT_SOURCE`，改用实际 `source`，环境变量覆盖时不再误导）。**验证**：重新生成后 `--check` 与「生成→再 `--check`」均退出 0 且无 diff。

**可选优化（一并做了 3 处）**：S1（`cap.Source` 此前只要命中 override 就标 `"override"`，即便该条目只给 Vendor；改为按「override 实际贡献了能力字段」标注，并补 `TestLookupCapabilityOverridePrefix`/`TestLookupCapabilityOverrideLevelsWin` 断言）；S2（覆盖层前缀语义在 `model_overrides.go` 头部加硬约束说明：优先完整 ID，前缀键要足够长，并补 `TestLookupCapabilityOverridePrefixSiblings` 记录该行为）；S3（剥离函数冗余条件已随 B1 修法收敛）。

**性能实测（审查 Agent 量化）**：84 条预编译正则 × 2000 模型（`MaxModels`）= 168000 次 `MatchString` ≈ **42ms**，单次 251ns；相对 `FetchModels` 的网络耗时可忽略，**无需前缀预筛等优化**。

**`GuessReasoning` 回归面（审查 Agent 独立复核）**：对预置表 212 个 id 双向比对，**零 `true→false`**；方向相反是 68 个 `false→true`（更准方向，但属**面向用户的可见行为变更**——导入后这些模型的「思考」开关默认状态可能由关变开）。这是内建能力表工作的预期结果，需在 `CHANGELOG.md` 记录（该文件由用户并行编辑，本轮未改）。

### 4.2 数据源（B2，按用户确认的「两路都要」）

> **本轮范围**：只实现用户明确要求的两路——**路线一 ZCode 正则** + **路线三 APM 自维护覆盖层**。原「路线二 DSH asar 目录」为可选增强，**本轮不做**（asar 解包脆弱、只覆盖精确 id、且 DSH 的档位本就是用户逐模型自填，收益低）。保留为后续可选项。

- **路线一 · ZCode 正则（家族/前缀维度，主数据）**：`scripts/sync-model-catalog.mjs` 从 `C:\AppData\Zcode\resources\config\provider\zcode-builtin.json` 抽取 **`modelRules`（84 条，按原顺序）** 的 `modelMatch` / `contextWindow` / `maxOutputTokens.max` / `reasoningLevel.values`，生成 `internal/core/model_catalog_gen.go`（每条规则预编译为包级 `*regexp.Regexp`）。文件头写「Code generated … DO NOT EDIT」，注释里写明源文件路径、`revision`、抽取日期与规则数。带 `--check`（对齐 `scripts/sync-version.mjs`）。
- **不进生成表的两张表**：`modelApiRules`（只有 `map` 没有 `values`，且必须结合 `apiTypeMatch` 才有意义）与 `providerSiteRules`（每条都绑定 `baseUrlMatch`，是**特定网关的调参**，脱离该网关不成立——抽进来会污染别的 provider）。两者作为文档化的后续扩展点保留。
- **路线三 · APM 自维护覆盖层**：`internal/core/model_overrides.go`，手写表，承载：脚本抽不到的家族（如 `deepseek-v4.1-flash` 若将来内建数据删掉）、APM 期望默认口径、对客户端数据的修正。
- **合并优先级**：`override` > `zcode`（正则，全表顺序覆盖）> 旧 `presets.go` 表（迁移兜底，保留但降级为最低优先）。

### 4.3 匹配器（B3）

`LookupModelCapability` 求值顺序：
1. **override 精确/前缀**命中即返回（可整体覆盖或逐字段覆盖）。
2. **zcode 正则全表顺序扫描 + 逐字段覆盖**（4.0 事实 2 的要求）：从空能力开始，凡规则 `modelMatch` 命中该模型名，就用该规则里**出现过**的 `contextWindow` / `maxOutputTokens.max` / `reasoningLevel.values` 覆盖当前值。
3. 仍缺则回落 `presets.go` 旧表（仅 ctx/output）。
4. `Vendor`：由独立的小型家族正则表（`anthropic|claude`、`gpt|o[34]`、`gemini`、`deepseek`、`glm`、`qwen`、`kimi|moonshot`、`minimax`、`grok`、`llama`、`mistral`、`doubao`、`hunyuan`…）判定，未命中 `unknown`。**不复用 ZCode 的 `config.group`**（已被 `mergeV2ProviderRule` 强写回 `standard-provider`，与厂商无关）。

性能：210 条正则若每次 `LookupModelCapability` 现编译会拖慢「导入 2000 个模型」的批量路径（`MaxModels = 2000`）。→ 生成文件里把每条规则编成**包级 `var` 预编译 `*regexp.Regexp` 数组**（`sync` 时生成 `mustCompile`），匹配走已编译对象；批量导入走 `ApplyModelPresets` 的循环即可。

### 4.4 消费点接入（B4）

- `cards.go` `BuildModelCards`：在 `autoFill` 分支里同时取 `cap.Context`/`cap.Output`（把现在直接调 `LookupModelLimits` 的那段换成 `LookupModelCapability`），并把 `cap.Vendor` 写进卡片新字段 `_vendor`、把 `cap.ReasoningLevels` 写进 `_supported_levels`（供阶段 C/D；JSON `omitempty`，未知则不带）。
- `cards.go` `GuessReasoning`：能力表有 `ReasoningLevels` 时以「集合里存在非 off 档」判定；否则回落 `infer.go` 关键词表（保持现状）。
- `models.go` `ModelCard`：新增
  ```go
  Vendor           string   `json:"_vendor,omitempty"`
  SupportedLevels  []string `json:"_supported_levels,omitempty"`
  ```
  两者均为 APM 计算值、**绝不写回配置**；`stripAPMInternalModelKeys` 需一并剥离（阶段 A 已有的剥除器里加这两个键）。
- `ApplyModelPresets`：`filled` 仍只统计 ctx/output 两类，保持既有前端提示语义不变（档位补全属于阶段 C，另立信号）。

### 4.5 验收（B5）

- 单测（新增 `internal/core/capability_test.go`）：
  - `deepseek-v4.1-flash` → ctx 1000000 / out 384000 / levels 含 `off` 且含 `high`（对应 `["disabled","low","high","max"]` 规范化）。
  - `glm-5.3-flash` → `Context==1000000`、`Output==128000`、`ReasoningLevels == ["low","high","max"]`（**不含 off**；验证逐字段覆盖继承 + 家族规则整体替换档位）。
  - `glm-5.3` → `ReasoningLevels == ["low","high","max"]`（不含 off）——阶段 A 的 `canToggleReasoning` 不可关逻辑要吃这个集合。
  - `glm-5.2` → `Context==1000000`（**只有大小写不敏感才拿得到**；含大写规则 `.*GLM-5\.2`），`minimax-m2.5` → `Context==204800`。
  - `gpt-5.6-luna-2026-01-01` → ctx 1050000（验证 `.*gpt-5\.6(?:-(?:sol|terra|luna))?` 家族命中）。
  - `kimi-k2.7-code` → levels `["enabled"]`；`kimi-k2.6` → `["off","enabled"]`。
  - `glm-4.6` → ctx 200000；`qwen3-max` → ctx 262144、**levels 为 nil（跳过兜底后无人给档位）**；`minimax-m2.5` → ctx 204800、levels nil。
  - **未知模型 `my-local-model` → 三字段皆空（ctx=0/out=0/levels nil）+ ok==true + Vendor=="unknown"**（验证 `.*` 兜底被跳过，不写入猜测值）；`deepseek-chat` 同样三字段皆空（它今天靠 `presets.go` 得到 128000/8000，属旧表兜底）。
  - 空档位集合统一置 nil 而非 `[]`（配合 `omitempty`）。
  - 覆盖优先级：override 表中同一个模型名的条目必须胜过正则结果（用可注入的 `lookupCapability(modelID, overrides)`）。
  - **逐字段覆盖的负向用例**：构造一条「后命中规则只给 `inputFormat`」的合成输入，断言 ctx/levels 不被清空（用可注入的 `mergeCatalog(modelID, rules)`）。
- **`GuessReasoning` 改走能力表的回归面（主会话实测，v4 补充）**：原关键词表要「模型名含 reason/think/o1/o3/o4…」才为 true；改后「能力表若给出档位集合则按集合判、否则回落关键词」。对 32 个代表性模型名双向比对，结果 **零个 `true→false` 回归**，只有 8 个 `false→true` 修正：`deepseek-v4-flash`、`deepseek-v4.1-flash`、`qwen3.8-omni-flash-*`、`gpt-5.6-luna`、`glm-4.6`、`glm-5.3-flash`、`kimi-k2.6`、`kimi-k2.7-code`——这些过去因模型名不含思考关键词而被误判为不支持思考，现在被内建档位数据正确识别。`deepseek-r1`、`qwq-32b`、`o1-preview`、`kimi-k2-thinking`、`glm-zero` 等仍走关键词表，结果不变。
- `node scripts/sync-model-catalog.mjs --check` 幂等（重跑无 diff）。
- 生成文件属「内建只读数据」：`init()` 阶段不得读真实用户文件，探测函数必须无副作用（`AGENTS.md`）。生成文件只含常量与 `regexp.MustCompile` 的包级 var，`init()` 不读盘。
- 不新增第三方依赖（`go.mod` 不动）；生成表只依赖 `regexp`。
- `go build ./... && go vet ./... && go test ./... -count=1` 全绿。

### 4.6 已决问题（替代「待确认」中与 B 有关的条目）

- **快照维护约定（原风险 3）**：采纳，但**闸门形态受限**——`AGENTS.md` 规定全仓库零 `os/exec`，`go test` 不能调用 `node`，因此**不新增 `TestModelCatalogInSync`**。改为：`sync-model-catalog.mjs --check` 作为提交前的手工闸门。生成文件里写上源 `revision` 与规则数，便于人工核对。**注**：`AGENTS.md` 当前正被用户并行翻译（工作区 `M`），故本轮**未改 AGENTS.md**；`--check` 门禁暂记在本节与 `TODO.md`，待用户决定何时并入「验证命令」清单。
- **隐私边界（原风险 4）**：脚本只读**应用自带资源**（`zcode-builtin.json`），脚本头注释显式声明来源、用途、不写回用户目录。
- **冲突口径（原风险 5）**：自维护覆盖层优先；v1 **不在 UI 标注每个值来源**（避免 UI 膨胀），来源只保留在 `Capability.Source` 供测试与调试。
- **DSH 仅精确 id（原风险 8）**：接受；由覆盖层兜底。
- **`map` 表达式（原风险 1）**：只读展示，不提供编辑。

## 5. 阶段 C — 档位自适应 + 高级开关（#2 的 UI 形态）

依赖阶段 B（`_supported_levels` 已由后端下发）。

**C0. 既有开关的去留（已决）**：**保留并重新定位** `reasoningAllIntensities`。它与 C1/C2 是三个正交关注点，不冲突：
- `reasoningAllIntensities`（既有）＝「打开思考时，种子集合用几分档」——开启时用全集 7 档，关闭时用基础三档 `off/high/max`。C1 落地后它的语义变为**「忽略能力表推断、强制预置全集」**（用户「我知道得比自动检测多」的逃生口）。
- C1 的「自适应启用」＝「能力表说该模型支持哪些档位」——决定**默认种子集合**。
- C2 的「高级自填」＝「允许写入任意档位名」——决定**校验是否放宽**。
三者优先级：C2 放宽校验 > C1 能力表种子 > `reasoningAllIntensities` 全量兜底。**不删除、不合并**。

**C1. 全集 + 自适应启用**
- 渲染：档位胶囊仍展示全集（7 档 + 集合含 `enabled` 时并入），但依据 `card._supported_levels` 区分「能力表支持」（默认高亮/启用）与「不支持」（呈未启用态，用户可手动点开）。
- 打开思考时的种子集合：`card._supported_levels` 非空 → 用它；为空 → 回落到 `reasoningAllIntensities ? 全集 : 基础三档`。
- 收敛三份硬编码档位表（`ModelCard.vue` / `ManagePage.vue` / `ImportPage.vue`）到 `constants/variants.ts`（阶段 A 已做，本轮只需在其上叠加「支持态」判定）。
- 文案要说明「支持集合为内建数据推断，可能滞后，可手动覆盖」。

**C2. 高级开关：自填档位**
- 新增后端设置项，落 `settings.json`，键 `custom_variants_enabled`（默认 false），落地点 6 处：`internal/app/app.go` 的 `Get/Set` 绑定 + `GetOptions()` 聚合 + 内部辅助 `customVariantsEnabled()`；`frontend/src/api/index.ts`；`stores/setting.ts`（`trackSetter`）；`OptionsPage.vue` 开关行；`App.vue` 启动回读。
- **放宽校验用包级开关**（照既有 `core.SetPrivateHostsAllowed` 模式）：新增 `core.SetCustomVariantsAllowed(bool)` + 包级 var；`NormalizeVariants`（`tokens.go`）在放行时不做白名单过滤（仍去重、仍 `canonicalVariant` 归一别名）；`CardToCfg`（`cards.go`）放行时不改写未知档位；DSH 侧 `dshModelToRaw` 白名单同步放宽。
- 打开后：模型卡露出自由输入（逗号/换行分隔），直接写 `reasoningLevel.values`。
- `map` 表达式：只读展示，不提供编辑。

**C3. 批量操作对齐**
- `ManagePage` 批量 pills 改为全集 + 支持状态标记；批量套用遇到不支持的档位时提示而非静默写入。

### 5.1 执行状态（阶段 C 已完成 2026-10-14）

**后端**：新增 `internal/core/variant_policy.go`（`SetCustomVariantsAllowed`/`CustomVariantsAllowed`，照 `urlutil.go` 的 `sync.RWMutex`+bool 模式）；`tokens.go` 的 `NormalizeVariants` 在开关打开时跳过白名单（仍去重 + 归一别名 + 去空 + **限长/禁控制字符**）；`cards.go` 的 `CardToCfg` 尊重开关、`CfgToCard` 补填 `Vendor`/`SupportedLevels`；`deepseek.go` 新增 `dshEffortForWrite`/`dshEffortForRead`（读/写对称）；`app.go` 新增 `Get/SetCustomVariantsEnabled` 绑定、`GetOptions()` 聚合、`Startup()` 同步、`customVariantsEnabled()`。
**前端**：`ModelCard.vue` 自适应档位渲染（`.pill.unsupported` 弱化不支持档位、支持集合做种子）+ 自填输入；`OptionsPage.vue` 新增开关行；`setting.ts`/`api/index.ts`/`App.vue` 打通后端持久化与启动回读；`ManagePage`/`ImportPage` 批量档位支持态标记 + `uiConfirm` 提示；i18n 八条新键三语齐全。

**集中验证（主会话）**：`go build`/`go vet`/`go test ./... -count=1` 与 `-shuffle=on` 全绿、`npm run build` OK、两个 `--check` OK、`go mod verify` OK、`git diff --check` 干净；全部改动文件 gofmt 干净（残留的 2 个 CRLF 提示是仓库既有现象）。

**过程补修（主会话发现）**：DSH **读端**原本仍按白名单过滤（`ConvertDeepSeekModel` 用 `dshCanonicalEffort`），开关打开后自填档位能写进配置但**重读即消失**，使「自填」实际不可用 → 新增 `dshEffortForRead` 并在读端两处（档位集合、默认档位）改用它，读/写对称；补 `TestDshCustomEffortReadWriteSymmetry` 与 ZCode 侧 `TestZCodeCustomVariantRoundTripSymmetry` 锁定。

### 5.2 独立代码审查轮（2026-10-14，只读 Agent）与修复

审查结论：**无阻断项**，两条应修 + 若干建议。主会话复核后处理：

- **应修 1（档位名无长度/字符校验）**：`NormalizeVariants` 放宽路径原本只要非空即放行。对照仓库既有约束（模型 ID 512、provider 名 256、options 64KB），唯独自填档位名无帽 → 用户可把超长/含控制字符的值落进真实配置（DSH 还会把它当 YAML 键）。**修法**：新增 `MaxVariantLen = 64`，`NormalizeVariants` 对**非白名单**档位额外做「限长 + 拒绝 ASCII 控制字符（`hasControlChar`）」；前端 `ModelCard.vue` 输入加 `:maxlength`、`commitCustomVariants` 同步过滤。补 3 条测试（超长拒绝 / 恰好等于上限保留 / 含控制字符拒绝）。
- **应修 2（批量提示文案与实现不符）**：新增文案称「批量仅作用于当前可见且被勾选的模型」，但实现（**阶段 A 之前就如此**）遍历的是 `filteredCards`，不含 `.select` 过滤。**修法**：修正文案为「批量作用于当前筛选出的可见模型」，使描述与行为一致（不改行为，避免动既有交互语义）。顺带修正 ja 文案里混入的中文字「档」（8 处，含 2 处既有）。
- **建议项**：`dshEffortForRead` 保留为语义别名并在注释里写明意图（读/写调用点自解释，将来分化时不改调用点）；`commitCustomVariants` 增加 `canonicalVariant` 等价归一（避免 `off`/`disabled`/`Disabled` 并存）；删掉 `@@` 转义注释（全仓库无实证，避免误导）。`CfgToCard` 逐卡 `LookupModelCapability` 的耗时经审查量化约 14µs/模型（2000 模型约 29ms），一次性读路径可接受，不做批量优化。

## 6. 阶段 D — 厂商分组视图（#3）

**D1. 厂商归属**：复用阶段 B 的 `Vendor`（家族正则），前端只吃后端下发的字段，不重复维护正则；未命中一律 `unknown`，分类失败不得影响模型编辑。
- 注意：`mergeV2ProviderRule` 会把 `config.group` 强制改回 `standard-personal`(`backend_zcode_v2.go:372-374`)，它与模型厂商无关，**不要**用 `config.group` 做厂商分类。

**D2. 视图**：模型列表加「按厂商分组」开关 + 厂商多选筛选；分组头显示厂商名、模型数、整组控件（整组开关思考 / 整组设档位）。
- 与既有交互的叠加语义要写清：`manageQuery`/`filteredCards`(`ManagePage.vue:317-321`) 与 `select`/`hasSelected`(`:323`) 同时生效时，**批量操作只作用于「当前可见且被选中」的集合**；分组只影响展示顺序。

**D3. 不写盘**：分类只在 APM 内部计算/缓存，绝不写入 `provider_config.json` 或 DSH 配置。

**D4. i18n**：`frontend/src/i18n/index.ts` 是 `zh/en/ja` **三语扁平字典**（331 条全部三语齐全，`t()` 缺键返回 key、缺语回退 zh）。新增键三语同补。

### 6.1 执行状态（阶段 D 已完成 2026-10-14）

**实现（纯前端，`ManagePage.vue` + i18n）**：
- 列表头部新增「按厂商分组」开关（复用全局 `.switch`，状态存 `localStorage: zcpm.vendorGroup`）+ 厂商多选筛选（`n-select multiple`，选项由当前模型集动态生成并带计数，`unknown` 恒排最后）。
- `filteredCards` **仍是扁平数组**（`editForm.cards` → 搜索过滤 → 厂商过滤），既有批量逻辑（思考开关/档位/默认档位/上下文/输出/删除所选）**作用域不变**；厂商筛选属于「可见性」，被筛掉的厂商不计入可见集合。
- 新增 `groupedCards`（按 `card._vendor` 分组、`unknown` 排最后）：仅在**渲染层**使用，分组开启时用 `v-for` 分组段 + 组内 `v-for` 卡片渲染；`TransitionGroup` 仍只保留在最外层，避免嵌套 FLIP 动画错乱。
- 分组头：厂商显示名（`vendorLabel` → i18n `vendor.*`）+ `{n} 个模型`；用既有 token（`--fluent-border`/`--fluent-text-soft`/`--fluent-card-bg`），sticky 顶部，无新配色。
- **不写盘**：分组/筛选全是本地视图状态，绝不进卡片数据或配置文件。
- i18n 新增 22 键（1 组开关 + 1 筛选占位 + 1 计数 + 19 个厂商标识），zh/en/ja 三语齐全、无 emoji。

**验证**：`npm run build`（vue-tsc + vite）通过、`npx vue-tsc --noEmit` 干净、`go test ./... -count=1` 全绿、`git diff --check` 干净、`wails build` 端到端通过（生成绑定 → 编译前端 → 编译应用），并确认 `vendor.groupToggle` 等标记已进入 `frontend/dist` 产物。
**未做**：真实 Wails 窗口的人工视觉核对（受本轮预算限制，未启动应用截图核对）；分组/筛选的视觉细节（间距、sticky 遮挡）建议人工过一遍浅/深两色主题。
**说明**：原计划派发的前端 Sub Agent 因账户余额不足失败（1.5 秒即退出、未产生任何改动），阶段 D 由主会话直接实现。

## 7. 待确认 / 风险

1. ~~**`map` 是否开放编辑**（C2）~~ **已决（4.6）**：只读展示。
2. **默认档位在 ZCode 侧无意义**（A-4）：确认是否直接从 ZCode 界面移除。**仍待用户拍板**；当前折中方案是保留控件 + 提示「仅对 DSH 生效」（阶段 A 已实现，见 `reasoning.zcodeDefaultNote`）。
3. ~~**快照维护约定**~~ **已决（4.6）**：`sync-model-catalog.mjs --check` 作为提交前手工闸门（因零 `os/exec` 约束，不新增 `TestModelCatalogInSync`）。
4. ~~**合理性与隐私边界**~~ **已决（4.6）**。
5. ~~**自维护层与客户端数据的冲突口径**~~ **已决（4.6）**：覆盖层优先，v1 不标注来源。
6. ~~**A-1/A-8 的修复依赖一条未证实的推断**~~ **已解决（v3）**：内建数据 275 处 `config.enabled` 实测证明它是「模型在列表中是否启用」，与思考无关；off 档位就是 `values` 里的一个词，`map` 用 `reasoningLevel == "disabled" ? {...disabled...} : {...enabled...}` 运行时求值。A-7 修法已据此更正，不再需要人工实测推断。
7. **C0 的开关重叠**：**待用户拍板**。建议方案——把 `reasoningAllIntensities` 的语义从「勾选多少档」升级为「打开思考时是否预置全部强度」，与 C1 的「按能力表自适应启用」并存但不冲突（前者管种子集合大小，后者管哪些档位可见/可点）；若用户选「替代」，则删除该开关，统一由能力表驱动。
8. ~~**DSH 目录只能按精确 id 匹配**~~ **已决**：本轮不做 DSH asar 路线，接受该限制。
9. **未得到独立验证的事项**（见附录 A）：app.asar 的两处实现差异、ZCode 版本号口径。
10. **新增风险（阶段 B，已缓解）**：内建 `revision=31` 的规则可能在后续 ZCode 升级中改变家族划分 → 用 `--check` 闸门 + 覆盖层兜底。~~`modelRules[0]` 那条 `.*` 兜底会把任意未知模型填成 ctx=200000~~ **已按设计解决（v4）**：实现里显式跳过 `Pattern == ".*"` 的纯兜底规则，未知模型保持「留空」，不再有「填了但不准」的问题（见 4.0b/4.0c）。
11. **匹配规则尚未补齐（用户 2026-10-14 指出「差一点规则」，留待后续对话，本轮不做）**：阶段 B 只抽了 ZCode `modelRules` 的 84 条家族规则，覆盖不到的那部分模型仍拿不到正确能力值。补齐方向三选（可组合）：
    - **ZCode 另外两张表**：`providerSiteRules`（54 条，**每条绑定 `baseUrlMatch` 网关**——脱离该网关不成立，抽进来会污染别的 provider，需带 baseURL 上下文才可用；当前有意未抽，见 4.2）与 `modelApiRules`（72 条，只有 `reasoningLevel.map` 没有 `values`，需结合 `apiTypeMatch` 才有意义）。
    - **APM 自维护覆盖层**：直接往 `internal/core/model_overrides.go` 追加（手写、优先级最高，前缀键要足够长以防误伤兄弟模型）。
    - **可选的 DSH pi-ai 目录**（原路线二，本轮未做）：`C:\AppData\DSH Desktop\resources\app.asar` 内 `dsh/node_modules/@earendil-works/pi-ai/dist/providers/data/*.json`，43 个 provider，字段含 `contextWindow`/`maxTokens`/`reasoning`/`thinkingLevelMap`；**只覆盖精确 id**（用户自命名模型匹配不到），抽取需按 asar 格式（header + offset）解包。
    **影响面**：只影响「自动补全命中率」，不影响既有正确性；实施后需重跑 `sync-model-catalog.mjs`（改内建表时）或直接编辑覆盖层，并补 `capability_test.go` 回归用例。**本条对应 `TODO.md` 第 18 项。**

## 8. 验证命令（每阶段收尾）

```text
cd frontend && npm ci && npm run build
cd ..
go test ./...
go vet ./...
go mod verify
wails build
git diff --check
```

前端改动另需人工检查亮/暗两套主题。

## 9. 必须遵守的硬性规则（阶段 C/D 会直接触发）

来自 `UI-GUIDELINES.md`：零 emoji（图标走 `@vicons/ionicons5`）；禁用原生 `prompt/confirm/alert`（用 `uiConfirm`/`uiPrompt`）；不新增 UI 库、不推翻既有骨架；颜色/圆角/时长/动效一律走 `styles/global.css` + `styles/theme.ts` 的 token；**错峰入场（`useStagger`）的序号不能空档**——分组与条件渲染会移动序号；复用既有 composable 与 `.pill`/`.badge` 等规范控件并补齐 a11y；同屏不得并存两套控件体系；「失败不得伪装成功、失败态不等于空态、异步要有占位」；i18n 三语齐全。

来自 `AGENTS.md`：只用相对导入（`@/*` 别名零使用）；跨组件共享用模块级 `ref`，不用 `provide`/`inject`；Wails 绑定不返回 `error`，一律 `{success, error, error_code}`，前端把 reject 与 `success:false` 都归一为 rejection；DTO 集中在 `models.go`、内部 meta 用 `_` 前缀、空切片不序列化为 `null`；依赖方向单向（`core` 不得引入 Wails、`internal/update` 不得依赖 Wails）；`init()` 阶段不得读真实用户文件、格式探测函数必须无副作用（B 的新生成文件必须遵守）；改动 `frontend/src/**` 前先读 `UI-GUIDELINES.md`。

## 10. 阶段 E — 模型能力维度补齐（harness 中立层）

> **状态：已实施（2026-10-15）**。E1（中立能力层 + 三 harness 映射）、E2（数据源加宽）、E3（前端能力区）
> 全部落地，E4 的集中验证与收尾在进行中；实施中的裁决与偏差逐条记录在 10.8。
> 触发问题：模型参数里除了上下文 / 输出 / 思考档位，
> 还能记录「是否支持图片 / 视频 / 音频 / PDF / 结构化输出 / 工具调用」等能力，用户要求「能自动填的全填」，
> 且**不局限于 ZCode v2**——DSH 与后续 harness 都要从同一层取值。
>
> 与此前各阶段的关系：阶段 B 建的是 **ctx / out / 档位** 三维的能力表（对应 `TODO.md` 第 18 项，已实现未提交）；
> 本节是把同一套分层机制**扩成超集能力维度**，并要求中立层不再泄漏 harness 专有键名。

### 10.0 已确认的决定（用户 2026-10-15 拍板）

1. **范围**：输入模态 5 位（文 / 图 / 视 / 音 / PDF）+ 输出模态 + 工具调用 + 结构化输出 + 原生联网搜索 + 对话中途 system。
2. **落盘策略**：**只写推断为真的位，从不写 `false`，从不写 `null`**。留空 = 让目标 harness 自己的默认值生效。
3. **数据源**：加宽 models.dev 快照。
4. **UI**：`ModelCard` 展开区加「能力」区 + 两页批量栏各加一行。
5. **架构**：能力数据是 **harness 中立**的一层，ZCode v2 / DSH / opencode 与后续 harness 都从同一层取值、各自映射。

### 10.1 事实基线（本轮已查实，带证据）

#### 10.1.1 ZCode 的合并语义（asar `/out/host/index.js` 实证）

- 内建 `modelRules` 在前、个人 `providerModelRules` 在后，`composeEffective` 拼接后**逐字段 overlay**。
- `overlayValue(t, n) = n === undefined ? t : n` ⇒ **省略键 = 不覆盖**；而 `overlayConfig` 里 **`null` = 显式清空**。
- 个人配置里的表名是 `providerModelRules` / `manualProviderModelRules`（**不是** `modelRules`），且 `.strict()`。
- 内建 `modelRules[0]` 是 `modelMatch: ".*"` 兜底规则，已填满 `properties` 全部 8 键 + `optionSpecs` 两键
  （ctx 200000 / out 32000 / `supportsText:true` / `supportsToolCall:true` / `supportsJsonSchemaOutput:false` …）。
- ⇒ **「只写真、留空即安全」成立**：省略键回落到内建 `.*` 的默认值，`validateComplete` 不报 issue，模型保持可选。
- ⇒ 反例风险只有三条：① 写 `null`（会被当清空）；② 写进 `manualProviderModelRules`
  （命中时会先 `clearManualModelConfig` 清空再覆盖，且该表 schema 是必填子集，写不全让整份个人配置解析失败）；
  ③ 写 `enabled`（会把模型从列表里关掉，现有 `convert.go:227-230` 已明确不写）。
- 既有实现已经刻意偏离内建兜底：`internal/core/capability.go:263-275` 的 `mergeCatalog` **跳过 `Pattern == ".*"`**，
  以免未知模型被填成猜测值（否则用户自命名的本地模型会被写进 200000/32000）。
  **新增能力位必须沿用同一跳过**，否则 `.*` 的 `supportsToolCall:true` 会占满所有模型、后面几层永远轮不到。

#### 10.1.2 ZCode v2 能力键全集（`internal/zcodeprovider/schema.go:176-212`、`order.go:56-75`）

| 维度 | 键 |
|---|---|
| 输入模态 5 位 | `properties.inputFormat.supports{Text,Image,Video,Audio,Pdf}` |
| 输出模态 | `properties.outputFormat.supportsText` |
| 工具调用 | `properties.supportsToolCall` |
| 结构化输出 | `properties.supportsJsonSchemaOutput` |
| 原生联网搜索 | `properties.supportsNativeWebSearch` |
| 对话中途 system | `properties.supportsMidConversationSystem` |
| mfjs 工具 schema | `properties.requiresMfjsToolSchema`（本轮不做） |
| 注入表达式 | `optionSpecs.reasoningLevel.map` / `maxOutputTokens.map`（本轮不做） |
| 模型启用位 | `config.enabled`（本轮不做） |

现状：`internal/zcodeprovider/convert.go:285-302` 的 `legacyInputFormat` **只**把 `modalities.input` 里的
image/video 映射成 `supportsImage`/`supportsVideo`，注释明写「不推断 text/audio/pdf，避免编造用户数据」。

#### 10.1.3 DSH 能力面（asar `dsh/node_modules/@deepseek-ai/dsh-llm-pi-ai/lib/index.js` 实证）

- 模型级 `modelFields`（`:1003-1010`）= `{name, contextWindow, maxTokens, input, reasoningEfforts, compat}`。
- **`MODALITIES = {text, image}`（`:279-282`），模型 `input` 只接受这两种**（`:1007`）；路由级另有
  `defaultInput`（`:1027`，默认 `["text"]`）。
- `compat` 配置侧只允许 27 键白名单（`:963-990`）。与能力相关者：`supportsStrictMode`（结构化输出，消费于
  `constrained-sampling.js:163-167`）、`supportsTemperature`（`anthropic-messages.js:143`）、`supportsStrictTools`。
- `assertOfferedCompatFields` / `COMPAT_GATES`（`:514-533, 609-711`）会**拒绝**配置里出现 catalog-only 的
  compat 键：`supportsToolSearch` / `supportsMidConvoSystemMessages` / `supportsOpenAIGrammarTools` /
  `supportsAdditionalTools` 等——它们只能来自内嵌 catalog。
- ⇒ DSH 可写入的能力仅：`input`（收敛到 text/image）、`compat.supportsStrictMode`、`compat.supportsTemperature`。
  工具调用 / 联网搜索 / 中段 system **在 DSH 配置层无法表达**（仍进中立层供展示与给别的 harness 用）。
- **现存缺陷（本轮顺带修）**：`internal/core/deepseek.go:635-639` 把 `card.RawCfg["modalities"]["input"]`
  **原样不过滤**搬进 DSH `input`；一旦含 video/pdf/audio 会被 DSH 的 schema 拒绝。必须收敛。

#### 10.1.4 opencode 能力面（`opencode_schema.json`）

模型条目 `modalities{input,output}` 的 enum 就是 `text|audio|image|video|pdf`（`:484-539`）；
`attachment`（`:363`）、`temperature`（`:369`）、`tool_call`（`:372`）为 boolean；`cost` 为 object。
APM 已透传 `modalities`/`attachment`（`cards.go:417-422, 471-486`），但**没有 UI、也没有从能力表填充**，
且 Go 侧不做取值域校验。

#### 10.1.5 中立层现状

- `internal/core/capability.go:26-41` 的 `ModelCapability` 只有 `Context/Output/ReasoningLevels/Vendor/Source` 五列，
  **无任何模态 / 工具位**。
- `internal/core/models.go:4-32` 的 `ModelCard.Modalities` 是**无类型 map**；`Attachment` 是 opencode 专用 bool。
- 离线目录编码 `capability_catalog.go:87-110` 为 `ctx|out|levels` 三段，解析要求**恰好三段**（fail-closed）。
- 生成脚本 `scripts/sync-model-catalog.mjs:10-11` 明确声明不抽 capabilities；
  `scripts/sync-ability-catalog.mjs` 只抽 `contextWindow/maxTokens/thinkingLevelMap` 与 models.dev 的 4 键，
  **主动丢弃** modalities / tool_call / structured_output / attachment / temperature。
- 仓库快照 `internal/core/data/models-dev-snapshot.json`（367KB）每个模型**只有 4 键**：
  `context / output / reasoning / levels`。

### 10.2 阶段 E1 — 中立能力层 + 三 harness 映射

#### 10.2.1 新增 `internal/core/model_features.go`

```go
// 全部三态：nil = 未知（不写盘）。modalities 存「已知支持」的正向集合。
type ModelFeatures struct {
    InputModalities  []string // 规范序：text, image, video, audio, pdf
    OutputModalities []string // 规范序：text, audio, image, video, pdf
    ToolCall         *bool
    StructuredOutput *bool // ZCode supportsJsonSchemaOutput / DSH compat.supportsStrictMode
    NativeWebSearch  *bool
    MidConvSystem    *bool
    Attachment       *bool
    Temperature      *bool // 能力档，非数值温度
}
```

- `NormalizeModalities(list)`：只留已知词表内取值、按固定规范序排序去重，空结果返回 nil。
- `mergeFeatures(dst, src)`：逐字段，`dst` 已有值则不改（与 ctx/out 的「前层胜出」同构）。
- `HasAny()`：供 UI / 日志判断。

配套：`ModelCapability` 增加 `Features ModelFeatures`；`modelOverride` / `catalogRule` / `siteRule` 各加 features 列；
`lookupCapabilityForSite` 在六个层里按同一顺序合并 features（`.*` 跳过语义不变）。

#### 10.2.2 卡片成为中立能力的载体

`ModelCard` 增加强类型字段（前端可直接编辑）：

```go
Capabilities *ModelCapabilities `json:"capabilities,omitempty"`
// input_modalities / output_modalities / tool_call / structured_output /
// native_web_search / mid_conv_system / attachment / temperature
```

- `BuildModelCardsForSite` / `CfgToCardForSite` 从 `LookupModelCapabilityForSite` 结果与 provider 自报字段
  共同填 `Capabilities`（**provider 自报优先**，沿用既有优先级）。
- **归并 opencode 形状**：读入时把既有 `attachment` / `modalities` 映射进 `Capabilities`；写出时 opencode 分支
  从 `Capabilities` 反向映射回 `modalities{input,output}` / `attachment`。**现有 opencode 往返测试必须原样通过**。
- 三个 writer 一律**读卡片的 `Capabilities`**，不各自猜键名。

#### 10.2.3 写盘约束（不变量）

| harness | 映射（只写真；键不存在则省略） | 明确不写 |
|---|---|---|
| ZCode v2 | `inputFormat.supports{Text,Image,Video,Audio,Pdf}`、`outputFormat.supportsText`、`supportsToolCall`、`supportsJsonSchemaOutput`、`supportsNativeWebSearch`、`supportsMidConversationSystem` | `null`、`false`、`enabled`、`requiresMfjsToolSchema`、`optionSpecs.*.map` |
| DSH | `input` ← `InputModalities ∩ {text,image}`（**空则整键省略**，不留空数组）；`compat.supportsStrictMode` ← StructuredOutput；`compat.supportsTemperature` ← Temperature | catalog-only compat 键；工具调用 / 联网搜索 / 中段 system（DSH 配置层无法表达） |
| opencode | `modalities{input,output}`（全词表）、`attachment`、`tool_call`、`temperature` | — |

落点文件：ZCode v2 在 `internal/zcodeprovider/convert.go`（`modelRuleFromLegacy`）与 `internal/core/backend_zcode_v2.go`；
DSH 在 `internal/core/deepseek.go` 的 `dshModelToRaw`；opencode 在 `internal/core/opencode.go` 与
`internal/core/cards.go` 的 `isOpenCode` 分支。

#### 10.2.4 冻结契约（实施前由主会话确认，实施中不得单方面改变）

**① 中立类型与 DTO（已落地，主会话所有）**

- `internal/core/model_features.go`：`ModelFeatures`（8 个能力列 + `Source`）、
  `NormalizeInputModalities` / `NormalizeOutputModalities` / `hasModality` / `IsEmpty` / `HasAny` /
  `featureBool` / `mergeFeatures`（前层胜出、后层填空缺）；`type ModelCapabilities = ModelFeatures`
  是同一结构的展示名，**不允许再定义第二份**。
- `internal/core/models.go`：`ModelCard.Capabilities *ModelCapabilities`（用户可编辑、写入端唯一数据源）
  与 `ModelCard.CapabilitiesInferred *ModelCapabilities`（`_` 前缀，仅供展示与「按能力表补全」，绝不写盘）。
- `internal/core/capability.go`：`ModelCapability.Features ModelFeatures`（求值链路填充）。
- `internal/core/capability_features.go`：生成表解析器，格式 `<input>|<output>|<flags>`，
  flags 固定序 `tjwmaT`（toolCall / jsonSchema / webSearch / midConv / attachment / temperature），
  每字符 `1`（已知为真）或 `-`；段数/长度不符即 fail-closed 返回零值。**与
  `ctx|out|levels` 那张表完全独立**（互不解析对方）。

**② 卡片 ↔ 写入端之间的内部通道 `_apm_capabilities`（冻结）**

`CardToCfg` 在**所有 kind** 下都往 legacy 中间形态写入（`stripInternalModelKeysInModels`
按 `_` 前缀统一剥离，因此它永远不会落盘）：

```json
"_apm_capabilities": {
  "input_modalities": ["text", "image"],   // 仅已知支持项，按规范序
  "output_modalities": ["text"],
  "tool_call": true,                        // 只写 true；未知/假一律省略该键
  "structured_output": true,
  "native_web_search": true,
  "mid_conv_system": true,
  "attachment": true,
  "temperature": true
}
```

- **读端**：`internal/zcodeprovider/convert.go`（v2 适配）读它映射到 `properties.*`；
  `opencode.go` / `deepseek.go` 拿到的是 `ModelCard`，直接读 `card.Capabilities`。
- **opencode kind 额外**写真实键（`modalities{input,output}` / `attachment` / `tool_call` / `temperature`），
  由既有 `isOpenCode` 分支负责；非 opencode kind 不写 `modalities`（保持既有行为，
  避免给 ZCode 3.14 legacy 文档塞它不认的键）。
- 只有「已知为真」的布尔位才出现；未知一律省略键，**绝不写 `false` / `null`**。

**③ 各 harness 的键映射（写端唯一口径）**

见 10.2.3 的表，补充两条实现细节：

- ZCode v2 的 `outputFormat.supportsText` 由 `output_modalities` 含 `text` 推出；
  `inputFormat.supports{Text,Image,Video,Audio,Pdf}` 由 `input_modalities` 逐项推出。
  同时把既有 `legacyInputFormat`（原只映射 image/video）扩到全部 5 词——
  它读的是**数据**而非推断，所以扩全不是编造。
- DSH 的 `compat` 必须与既有 compat **合并**（不整块替换）；`input` 收敛到
  `{text,image}` 的交集，交集为空则**整键省略**（不留空数组）。

**④ 生成物边界**

- 新增 `internal/core/capability_features_gen.go`（`map[string]string`，只收录有能力的 id），
  由 `scripts/sync-ability-catalog.mjs` 生成；`capability_catalog_gen.go` /
  `model_catalog_gen.go` / `model_site_rules_gen.go` 的既有编码**不得改变**。
- `model_catalog_gen.go` / `model_site_rules_gen.go` 的 `catalogRule` / `siteRule` 结构体可新增
  features 列（由主会话在求值链路侧定义），但**必须保持既有字段名与顺序**，
  生成脚本按新增列追加即可；旧解析契约（恰好三段）不动。

### 10.3 阶段 E2 — 数据源加宽

**`scripts/sync-model-catalog.mjs`**：`modelRules` 与 `providerSiteRules` 抽取新增能力位（inputFormat 5、outputFormat、
toolCall、jsonSchemaOutput、nativeWebSearch、midConversationSystem）；仍**不抽** `requiresMfjsToolSchema` /
`optionSpecs.*.map` / `config.enabled`。

**`scripts/sync-ability-catalog.mjs`**：

- pi-ai：读 `input[]`（text/image）与 `compat` 子集（脚本内维护**显式白名单映射表**，如
  `supportsStrictMode` → 结构化输出、`supportsTemperature`、`supportsMidConvoSystemMessages`）。
- models.dev：`buildSnapshot` 加宽为 `modalities{input,output}`、`tool_call`、`structured_output`、`attachment`、
  `temperature`。需在开发者机跑**一次** `--update-snapshot`（联网）重裁快照；此后 `--check` 仍完全离线幂等。
- **编码决策**：新增**并列产物** `internal/core/capability_features_gen.go`（`map[string]string`，值形如
  `text,image|text|1-0--1`，flags 固定顺序 `t(oolCall) j(sonSchema) w(ebSearch) m(idConv) a(ttachment) T(emperature)`，
  每字符 `1`/`0`/`-`），**不动**现有 2303 行 `ctx|out|levels` 与它「恰好三段、fail-closed」的解析器及测试。
  理由：零风险、`--check` diff 最小、旧解析契约不破。只收录「有能力的 id」。

**`internal/core/model_overrides.go`**：条目可选带 features 列，供手工纠偏。

### 10.4 阶段 E3 — 前端

- `frontend/src/types/index.ts`：加 `capabilities` 类型（与后端 DTO 对齐）。
- `frontend/src/components/ModelCard.vue` 展开区新增「能力」区：
  - 输入模态 5 个 `.pill` 多选（复用档位胶囊写法）；输出模态按 opencode 词表多选。
  - 6 个布尔位（工具调用 / 结构化输出 / 原生联网搜索 / 中段 system / 附件 / 支持温度）：`.check`。
  - 推断来源弱化提示（仿既有 `_supported_levels` 的 note 写法），推断值可被用户覆盖。
- 批量栏：`frontend/src/pages/ManagePage.vue` 与 `ImportPage.vue` 各加一行「能力」——按当前模型集动态生成
  （带计数）的多选 `n-select` + 「按推断填充」按钮，走能力表。选型逻辑仿 `composables/useVendorGroup.ts`。
- `frontend/src/i18n/index.ts`：新增键，**zh/en/ja 三语齐全、零 emoji、不硬编码中文**。
- 复用既有层（`.pill` / `.check` / `.field` / `.batch-pill` / `PageHeader` / `StickySaveBar`），不新增 UI 库。

### 10.5 阶段 E4 — 测试与验证

- **中立层**：逐字段覆盖、`.*` 跳过、只写真、never-null。
- **ZCode v2**：部分 `inputFormat` 对象被接受、读回往返；断言输出里**永不出现** `null` / `enabled` /
  `requiresMfjsToolSchema` / `*.map`。
- **DSH**：`input` 收敛到 `{text,image}`、空则省略；`compat` 只用 27 键白名单。**新增反例测试**：喂含 video/pdf 的
  modalities，断言被收敛且不产生非法值。
- **opencode**：modalities / attachment / tool_call / temperature 往返。
- **零回归**：把现有 2376-id 契约（`internal/core/capability_regression_test.go`）扩到 features——旧层已给的 features
  不得被新层改写，并保留反空断言。
- **生成物**：两个 `--check` 离线幂等；快照重裁后在无网环境可复现。
- **端到端**：`go build ./...`、`go vet ./...`、`go test ./... -count=1`、`go mod verify`、`cd frontend && npm run build`、
  `wails build`、`git diff --check`；对 CMD GOAT 猫猫 / 秋雪（90 模型）复测能力覆盖率。
- **人工**：浅色与深色两套主题目测。

### 10.6 不做 / 不变量

- 不写 `null`（ZCode overlay 会当清空）、不写 `false`、不写 `enabled`、不写 `requiresMfjsToolSchema`、
  不写 `optionSpecs.*.map`。
- 不改任何写入路径（`PrepareChange` / `WriteConfigBytes` / `guardZCodeV2Doc`）；不新增运行时网络
  （快照重裁只在开发者机手动触发）。
- **价格 / 知识截止 / 发布日期 / 开源权重**本轮不做（无 harness 落点）；models.dev 快照也不加这几列，
  后续要加是独立小改动。
- 不触碰真实用户配置与凭据；测试一律占位值 + `t.TempDir()` / `t.Setenv` 沙箱。

### 10.7 涉及文件清单

- **新增**：`internal/core/model_features.go`、`internal/core/model_features_test.go`、
  `internal/core/capability_features_gen.go`（生成物）。
- **修改**：`internal/core/capability*.go`、`cards.go`、`models.go`、`model_overrides.go`、`backend_zcode_v2.go`、
  `internal/zcodeprovider/convert.go`、`internal/core/deepseek.go`、`internal/core/opencode.go`、
  `internal/core/data/models-dev-snapshot.json`（重裁）、`scripts/sync-ability-catalog.mjs`、
  `scripts/sync-model-catalog.mjs`、`frontend/src/types/index.ts`、`frontend/src/components/ModelCard.vue`、
  `frontend/src/pages/ManagePage.vue`、`frontend/src/pages/ImportPage.vue`、`frontend/src/i18n/index.ts`、
  `CHANGELOG.md`、`TODO.md`、本文件。

### 10.8 实施记录与偏差（2026-10-15）

1. **契约层先由主会话写死**（`model_features.go`、`capability_features.go`、`models.go` 的 DTO 字段、
   `ModelCapability.Features`），再按**文件归属互不重叠**并行五路实施（求值链路 / 卡片层 /
   harness 适配 / 数据脚本 / 前端），主会话集中编译、跑测、复核。这样每路都能独立编译，
   不需要跨文件协商键名。
2. **模态的合并语义分两层**：**跨层**仍是「前层胜出、后层填空缺」（`mergeFeatures`）；
   但**同一张生成表内部**，pi-ai 与 models.dev 对同一 id 的模态取**并集**、布尔位取「或」——
   pi-ai 的 `MODALITIES` 词表只有 `text,image`，它给不出 pdf/video/audio 并不等于否定它们。
3. **卡片层「只记真」**：provider 明确说 `false` 时**丢弃**（不记成 `false`）。理由：中立层的写盘策略
   是「只写真、留空让目标 harness 的默认值生效」，记下 `false` 既没有落点，又容易被展示误读成
   「已确认不支持」。三态里「已知为假」只可能来自用户手改。
4. **`_capabilities_inferred` 加 `HasAny()` 守卫**：推断不出任何已知位时不填该字段，
   避免每张未知模型卡片都带一个空对象。
5. **「自动匹配」顺带补能力位**（`ApplyModelPresetsForSite`），语义为**只补空缺**：ctx/out 是估算值
   可以覆盖，能力位可能已被用户逐位确认过，覆盖等于抹掉用户选择。`filled` 仍只统计 ctx/out。
6. **修掉 DSH 两处内部键落盘泄漏**（实施中发现，均属既有同类问题）：旧式顶层 `provider` 布局
   `deepSeekUpsertEntry` 原样并入编辑载荷的 `models`；`deepSeekBuildEntry` 在合并语义下经
   `overlayModelNode` 保留旧节点里未被覆盖的键。现在前者走 `stripAPMInternalModelKeys`，
   后者在写出口对每个模型节点统一剥下划线前缀键（`_apm_capabilities` 与既有的 `_dsh_raw` 一并覆盖）。
7. **快照重裁的连带漂移**：`models.dev` 快照加宽（367KB → 803KB）会让
   `capability_catalog_gen.go` 出现 6 行上游漂移（4 行表头计数 + 2 行改值 + 2 行新增，
   例 `agnes-3.0-flash` context 524288→512000、`qwen3.6` 档位由 5 档变空）。这些是**上游数据本身的变化**，
   不是编码改动；不接受漂移就只能放弃快照加宽（能力位表会少一半来源），故保留并在此记录。
8. **站点层不跳过 `.*`**：只有 `catalogRule.Pattern == ".*"` 那条内建兜底要跳（跳过它是 APM 的既有契约），
   站点规则里的 `.*` 模型名是 ZCode 自己写死的站点级规则，照常参与。
9. **测试前提改为注入合成表**：凡是「某层给不出东西」才能成立的用例，一律注入合成表/合成 id，
   而不是断言真实生成表里没有某个 id——生成物会随快照重裁变化，这样重裁时不会出现假失败。
10. **内部键落盘的第三个出口也补上了**：`ImportOpencodeProviders` 的产物由 `App.ImportOpencode`
    经 `core.WriteConfig` 直写（不过任何 backend 的 `Store.Upsert`），E1 在中间形态里新增的
    `_apm_capabilities` 会顺着它落盘（既有的模型级/提供商级 `_opencode_raw` 早就如此）。
    该入口在 UI 上已隐藏、当前不可达，但 Wails 绑定仍在，故照样修：出口处按同一口径剥 `_` 前缀键。
11. **已知但未修的潜在出口（记录在 `TODO.md` 第 21 项）**：`app.go` 的 `SaveProvider` /
    `ImportProvider` 在「没有注册 backend」时的兜底分支会直写 `CardsToCfgMap` 的产物且不剥离内部键。
    该分支当前不可达（两个可选 Agent 的 `GetBackend` 恒有解），且会连带泄漏 `_raw_cfg` /
    `_off_alias` 等**既有**内部键，属存量隐患，不在本轮改动面内。

## 附录 A：证据与未能独立验证的事项

- 数据来源：本机安装的 ZCode（内建数据 `schemaVersion=1 / revision=31`；据 `app.asar/package.json` 为 **3.15.3**，早期笔记记为 3.15.1，不影响任何结论）与 DSH Desktop `0.2.0-rc.2`（`app-update.yml` 显示 `channel: nightly`）。全程只读应用自带资源。
- 验证方式：两个只读 agent 各自独立复核（一个走 Go 读写链逐步仿真，一个重算内建数据统计并复核前端行号），本次主会话另独立确认了 DSH 内嵌 pi-ai 目录的存在。
- **未能独立验证**：`resolveLegacyReasoningLevel` 的两处实现差异（仅来自 app.asar 静态阅读，无运行时分枝证据）；ZCode 版本号的两种说法；「values 不含 off 名时 ZCode 回落 `disabled`」这一行为（属推断，见风险 6）。
