#!/usr/bin/env node
// 模型能力目录同步工具：从 ZCode 应用自带的 provider 资源中抽取模型规则，
// 生成 internal/core/model_catalog_gen.go，供 APM 做模型能力推断。
//
//   node scripts/sync-model-catalog.mjs          就地生成 internal/core/model_catalog_gen.go
//   node scripts/sync-model-catalog.mjs --check  只校验；发现不一致时以非零码退出
//
// 数据来源：ZCode 应用自带资源 config/provider/zcode-builtin.json（只读，绝不写回）。
// 主产出 modelRules 数组保持原始顺序；追加产出 providerSiteRules（站点级规则，见文件和
// 脚本后半段的说明）。不抽取 modelApiRules、templateModelRules、builtinProviderModelRules：
// 它们或只有 map 无关值、或每条绑定 providerId 而非 baseUrl，不适用于 APM 的本地能力推断。
//
// 两条产出都带 Features 列（能力位紧凑串，编码与白名单见下方「能力位抽取」一节）。
// 仍然**不抽** requiresMfjsToolSchema、optionSpecs.*.map、config.enabled：前者是 mfjs 引擎
// 内部约定、后两者是注入表达式与启用开关，都不是「模型能力」，写进中立层会误导。
//
// 本脚本不读写任何用户目录，不读取任何凭据，不修改用户配置。可用环境变量
// APM_ZCODE_BUILTIN 覆盖内置资源路径（默认见 DEFAULT_SOURCE）。
import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const checkOnly = process.argv.includes('--check')

// 默认路径固定为 ZCode 应用自带资源；可用环境变量覆盖以便测试或换装位置。
const DEFAULT_SOURCE = 'C:\\AppData\\Zcode\\resources\\config\\provider\\zcode-builtin.json'
const source = process.env.APM_ZCODE_BUILTIN || DEFAULT_SOURCE
const target = 'internal/core/model_catalog_gen.go'
// 追加产出：站点级能力规则表（providerSiteRules）。本文件读者是 internal/core/model_site_rules.go。
const siteTarget = 'internal/core/model_site_rules_gen.go'

if (!existsSync(source)) {
  console.error(`未找到 ZCode 内置 provider 资源：${source}`)
  console.error('请确认 ZCode 已安装，或用环境变量 APM_ZCODE_BUILTIN 指定该文件路径。')
  process.exit(2)
}

let parsed
try {
  parsed = JSON.parse(readFileSync(source, 'utf8'))
} catch (e) {
  console.error(`解析内置 provider 资源失败：${source}`)
  console.error(String(e && e.message ? e.message : e))
  process.exit(2)
}

const modelRules = parsed?.config?.modelConfigRules?.modelRules
if (!Array.isArray(modelRules)) {
  console.error('内置资源缺少 config.modelConfigRules.modelRules 数组，无法抽取。')
  process.exit(2)
}

const rawRevision = parsed?.revision
const revision = Number.isInteger(rawRevision) ? rawRevision : 0

// 单条规则的最后一段路径存在即为整数；不存在或类型不对一律归零，与 Go 侧约定一致。
const intOf = (v) => (typeof v === 'number' && Number.isFinite(v) ? Math.trunc(v) : 0)

// ---- 能力位抽取 ----------------------------------------------------------------
//
// 编码是 `<input>|<output>|<flags>` 三段（与 internal/core/capability_features.go 的
// parseFeatureEncoding 共享契约，两侧任何改动都必须同步）：
//   input   逗号分隔的输入模态，按 inputModalityOrder 规范序（text,image,video,audio,pdf）；
//   output  逗号分隔的输出模态，按 outputModalityOrder 规范序（text,audio,image,video,pdf）；
//   flags   **恰好 6 个字符**，固定序 tjwmaT，每字符 '1'（已知为真）/ '-'（未知）。
// 这里对 ZCode 内建资源只可能点亮 t（supportsToolCall）、j（supportsJsonSchemaOutput）、
// w（supportsNativeWebSearch）、m（supportsMidConversationSystem）与模态两段：内建资源里
// 根本没有附件位（a）与温度支持位（T），所以这两字符恒为 '-'，**不猜**。
//
// 只把 `true` 编成 '1'：显式 `false` 与键缺失在编码上都落成 '-'。为什么把 false 也当 '-'？
// 因为本表只承载「已知为真」的正向事实，解析端 '0' 与 '-' 等价（都表示未知、不落盘），
// 多记一个 '0' 只会让生成物变长而没有任何消费方。模态两段的取词规则见下面 IMPLIED_TRUE_KEYS：
// **输入段才有「text 缺键即真」这一个例外，而且以 inputFormat 整段存在为前提**（段缺失 =
// 这一层不参与输入面，保持空段）；输出段没有例外（这个不对称是刻意的，理由见该常量注释）。
const FEATURE_FLAG_ORDER = 'tjwmaT'
const INPUT_MODALITY_FIELDS = [
  ['text', 'supportsText'],
  ['image', 'supportsImage'],
  ['video', 'supportsVideo'],
  ['audio', 'supportsAudio'],
  ['pdf', 'supportsPdf'],
]
// ZCode v2 的输出面只有 outputFormat.supportsText 一位。
const OUTPUT_MODALITY_FIELDS = [['text', 'supportsText']]

// IMPLIED_TRUE_KEYS 是「**缺键即默认支持**」的能力键，且**只在输入方向、且只在输入段存在时生效**。
//
// 输入侧用它的依据是内建资源自身的写法：ZCode 的输入面能表达全部 5 种模态，语义是「text 是
// 默认项，其余模态都是在 text 之上追加的增量」——modelRules 84 条里唯一写了
// inputFormat.supportsText 的正是兜底规则 `modelMatch: ".*"`，而且它写的是 true；其余 83 条
// 只写 supportsImage/Video/Audio/Pdf。也就是说 ZCode 只把**非默认项**写出来，缺键表示「沿用
// 默认」，不是「未知、更不是不支持」。providerSiteRules 是同一套写法。若照搬严格的 === true，
// 抽出来的输入模态会丢掉 text（例如只有 supportsImage: true 的规则会得到 `image`），而下游
// 中立层对模态是整字段「前层胜出」，这个残缺集合会把离线目录（models.dev）给出的完整集合整段
// 挡住，最终让 DSH 写出 `input: [image]`——那在 DSH 里是「不接受文本输入」的完整集合语义。
//
// **但「缺 text 键」与「缺整个 inputFormat 段」是两回事，必须分开**：
//   - `inputFormat` 是对象 ⇒ 这一层声明了输入面，里面缺 supportsText 才等于「沿用默认 true」；
//   - `inputFormat` 整段缺失（含 null / 非对象）⇒ 这一层**完全不碰输入面**，没有任何输入模态可抽。
// 依据是 ZCode 的 overlay 是**逐键**语义（ModelInputFormatConfig.overlay 对
// supportsText/Image/Video/Audio/Pdf 逐个 overlayValue：`n === void 0 ? keep : n`）——省略某
// 个键 = 不覆盖该键，整段缺失 = 五个键一个都不参与。整段缺失时若仍按 implied 产出
// `text||…`，就是把「这一层没说话」翻译成「输入只有 text」，而它在分层求值里会按「前层胜出」
// 收窄整条输入面（实测 `api.z.ai/api/anthropic` + anthropic-messages 这类协议变体条目会把
// 目录里上千个 id 的输入收窄成 text）。这个口径与 v2 读端完全一致：zcodeprovider 的
// InputFormatModalities 在 `format == nil` 时返回 nil，只有段存在时才按 implied 读 text。
// 实测 providerSiteRules 54 条里 23 条整段没有 inputFormat（都是后置的协议变体条目，只带
// 标志位），modelRules 里 2 条（`.*\[1m\]`、`.*(?:ox-alpha|glm-x-preview-f|x-preview-f-free).*`）。
//
// **输出侧刻意不对称，必须保持严格 `=== true`**：ZCode 的输出面只有
// `outputFormat.supportsText` 这一位，它**根本无法表达 audio / image / video / pdf 输出**。
// 「输出缺键即默认 text」一旦按整字段胜出，就会用「输出只有 text」抹掉下层对非文本输出模型的
// 正确读数。实测两个受害者：`mimo-v2.5-tts` 被家族规则 `.*mimo-v2\.5(?:[.\-:/\[].*)?` 命中，
// 而离线目录给的是 `text|audio`；`gpt-6-astra` 被家族规则 `.*gpt-6-astra(?:[.\-:/\[].*)?` 命中，
// 目录给的是 `text,image,pdf|text,image`。这两条规则整段都没写 outputFormat，因为输出侧除了
// text 它没有别的词可写——缺键只说明「这条规则没提输出」，不构成「输出只有 text」的证据。
// 输入侧则相反：缺 text 时它完全有能力写别的模态，只是把默认项省略了。
// 严格语义下输出段只会由显式写了 supportsText: true 的规则产生（内建兜底规则 `.*` 就是），
// 因此「输出段非空必含 text」这条不变量依然成立。
//
// supportsImage/Video/Audio/Pdf 与四个能力位（supportsToolCall / JsonSchemaOutput /
// NativeWebSearch / MidConversationSystem）一律维持严格的 === true：它们是叠加在 text 之上的
// 增量，缺键只说明这条规则没提到它，把它当「支持」就是凭空猜测。
const IMPLIED_TRUE_KEYS = new Set(['supportsText'])

// extractFeatures 把一条规则 config.properties 里的能力位编成紧凑串。
// 无任何已知为真的位时返回空串——**只有真的有能力位才写非空串**，这样生成物的每一行
// 要么带真实信息、要么保持最小的空串，跨次重生成不会产生噪音 diff。
//
// 有意不抽：requiresMfjsToolSchema、optionSpecs.*.map、config.enabled（见文件头说明）。
function extractFeatures(properties) {
  const props = properties && typeof properties === 'object' ? properties : {}
  // inputFormat 整段缺失（含 null / 非对象）时保留 null：这一层完全不碰输入面（见 IMPLIED_TRUE_KEYS）。
  const inputFmt = props.inputFormat && typeof props.inputFormat === 'object' ? props.inputFormat : null
  // 输出段没有 implied 键（严格 === true），因此「缺段」与「空段」结果相同，不需要这个区分。
  const outputFmt = props.outputFormat && typeof props.outputFormat === 'object' ? props.outputFormat : {}
  // 已知为真 = 显式 true，或（**仅当 implied=true 时**）「缺键即默认支持」的键
  // （IMPLIED_TRUE_KEYS，目前只有 text 这位）且未被显式写成 false。
  // 输入段传 implied=true、输出段传 false——这个不对称是刻意的，理由见 IMPLIED_TRUE_KEYS 注释。
  // words 按字段表的规范序过滤，因此 text 补进来时仍在首位。
  const known = (source, key, implied) =>
    source[key] === true || (implied && IMPLIED_TRUE_KEYS.has(key) && source[key] !== false)
  const words = (fields, source, implied) => fields.filter(([, key]) => known(source, key, implied)).map(([word]) => word)
  // 输入段：段不存在（null）⇒ 空段；段存在 ⇒ 内部缺 supportsText 键按 implied 补 text。
  const input = inputFmt === null ? [] : words(INPUT_MODALITY_FIELDS, inputFmt, true)
  const output = words(OUTPUT_MODALITY_FIELDS, outputFmt, false)
  const flags = [
    props.supportsToolCall === true, // t
    props.supportsJsonSchemaOutput === true, // j
    props.supportsNativeWebSearch === true, // w
    props.supportsMidConversationSystem === true, // m
    false, // a：内建资源没有附件位
    false, // T：内建资源没有温度支持位
  ]
  if (input.length === 0 && output.length === 0 && !flags.some(Boolean)) return ''
  return `${input.join(',')}|${output.join(',')}|${flags.map((b) => (b ? '1' : '-')).join('')}`
}

// Features 是固定字符集的短串（模态词、逗号、竖线与 1/-），不含反引号与引号，
// 因此统一用双引号字面量，和生成物里的示例写法 `Features: "text,image|text|1-1---"` 一致。
const featuresLit = (features) => JSON.stringify(features)

// 空 features 时**整段省略字段**（Go 侧零值就是 ""，语义完全等价）：生成文件最小、diff 最小。
// 只有真的抽到能力位的那几条规则才会多出 `, Features: "..."` 这一段，跨次重生成不会把
// 全表行都刷一遍。
const featuresField = (e) => (e.features === '' ? '' : `, Features: ${featuresLit(e.features)}`)

// ---- 生成物自检 ----------------------------------------------------------------
//
// 两个计数器：整段没有 inputFormat 的规则条数（生成模式下会打出来），以及自检失败次数。
let inputFormatMissingCount = 0
let validationFailures = 0

// checkMissingInputFormat 核对「源数据里 inputFormat 整段缺失的规则，产出里不能有输入模态段」
// （依据见 IMPLIED_TRUE_KEYS 的整段缺失说明）。这里刻意**重新读源数据**判断，不复用
// extractFeatures 的内部判定：抽取逻辑一旦被改回「整段缺失也 implied text」，这条闸门立刻报错。
function checkMissingInputFormat(where, rule, features) {
  const raw = rule && rule.config ? rule.config.properties : null
  const segmentPresent = !!raw && typeof raw.inputFormat === 'object' && raw.inputFormat !== null
  if (segmentPresent) return true
  inputFormatMissingCount++
  const inputSegment = features === '' ? '' : String(features).split('|')[0]
  if (inputSegment !== '') {
    console.error(
      `${where}中有规则缺少 inputFormat 段（${JSON.stringify(rule?.modelMatch ?? rule?.baseUrlMatch ?? '')}），` +
        `整段缺失表示这一层不参与输入面，却产出了输入模态 "${inputSegment}"：${features}`
    )
    return false
  }
  return true
}

// checkEncodingShape 校验产出的编码形状（与 internal/core/capability_features.go 的
// parseFeatureEncoding 共享契约）：要么是空串（整条省略 Features 字段），要么恰好三段、flags
// 恰好 6 位，且**至少有一个已知为真的位**——三段全空/全 '-' 的占位串没有任何信息，按契约必须
// 整条省略，否则只会制造噪音 diff。
function checkEncodingShape(where, features) {
  if (features === '') return true
  const segments = String(features).split('|')
  if (segments.length !== 3 || segments[2].length !== FEATURE_FLAG_ORDER.length) {
    console.error(`${where}的 Features 编码形状非法（应为 <输入>|<输出>|6 位 flags）：${features}`)
    return false
  }
  if (segments[0] === '' && segments[1] === '' && !segments[2].includes('1')) {
    console.error(`${where}的 Features 编码没有任何已知为真的位（应整条省略该字段）：${features}`)
    return false
  }
  return true
}

// 内建的兜底规则 `modelMatch: ".*"`（表里第一条）带着满格能力位（supportsText /
// supportsToolCall / outputFormat.supportsText 皆为 true）。本脚本**照常抽取**它——它确实
// 是内建资源的事实；「跳过 `.*`」是**求值链路**的决定（internal/core/capability.go 的
// mergeCatalog 有意跳过 Pattern == ".*"，否则任何未知模型都会被兜底填成猜测值）。把跳过
// 放在运行时而不是生成期，是为了让生成物忠实反映源数据，且不改变既有两条产物的字节结果。

const entries = []
let ctxCount = 0
let outputCount = 0
let levelsCount = 0
let backtickFallback = 0
let featuresCount = 0

for (const rule of modelRules) {
  const pattern = typeof rule?.modelMatch === 'string' ? rule.modelMatch : ''
  const context = intOf(rule?.config?.properties?.contextWindow)
  const output = intOf(rule?.config?.optionSpecs?.maxOutputTokens?.max)
  const rawLevels = rule?.config?.optionSpecs?.reasoningLevel?.values
  // 原样保留字符串切片，不做任何规范化；缺失即为 nil。
  const levels = Array.isArray(rawLevels) ? rawLevels.filter((s) => typeof s === 'string') : null
  const features = extractFeatures(rule?.config?.properties)

  if (context !== 0) ctxCount++
  if (output !== 0) outputCount++
  if (levels !== null) levelsCount++
  if (features !== '') featuresCount++
  // 生成物自检（见上方「生成物自检」一节）：整段缺 inputFormat 的规则不得带输入段，编码形状合法。
  if (!checkMissingInputFormat('modelRules', rule, features)) validationFailures++
  if (!checkEncodingShape('modelRules', features)) validationFailures++

  // Pattern 保留 ZCode 原始拼写；Re 需要与 ZCode 内部分配器行为一致：
  // 全串锚定（^(?:...)$）且大小写不敏感（(?i)）——provider 返回的模型名通常是小写。
  const regex = `(?i)^(?:${pattern})$`

  // 用反引号原始字符串；实测内置资源中不含反引号。若出现则退回双引号字面量。
  const lit = (s) => {
    if (s.includes('`')) {
      backtickFallback++
      return JSON.stringify(s)
    }
    return '`' + s + '`'
  }

  entries.push({ patternLit: lit(pattern), regexLit: lit(regex), context, output, levels, features })
}

// EOL 与仓库保持一致：以现有生成文件为准，缺省 CRLF。
const eolOf = (raw) => (raw.includes('\r\n') ? '\r\n' : '\n')
const eol = existsSync(join(root, target)) ? eolOf(readFileSync(join(root, target), 'utf8')) : '\r\n'

// 文件头**不写抽取日期**：--check 做整文件字节比较，若头部含「今天」则跨天后即便内置
// 资源一条未变也会误报不一致（不幂等）。改为只记录源 revision 与实际使用的源路径，
// 二者都只随真实数据变化。
const levelsLit = (levels) =>
  levels === null ? 'nil' : '[]string{' + levels.map((s) => JSON.stringify(s)).join(', ') + '}'

const lines = [
  '// Code generated by scripts/sync-model-catalog.mjs from zcode-builtin.json. DO NOT EDIT.',
  '',
  'package core',
  '',
  'import "regexp"',
  '',
  '// 由 scripts/sync-model-catalog.mjs 抽取，源文件：',
  `//   ${source}`,
  `// 源 revision: ${revision}，规则条数: ${entries.length}`,
  '// 每条规则的 Context/Output 为 0 表示该规则未给出该字段；Levels 为 nil 表示未给出。',
  '// 匹配语义是「全表顺序扫描、逐字段覆盖」：后命中规则里出现过的字段覆盖先前的值，未出现的字段保持不变。',
  '// 文件头故意不写抽取日期，以保证 --check 跨天幂等（见脚本内注释）。',
  '',
  'var modelCatalogRules = []catalogRule{',
]

for (const e of entries) {
  lines.push(
    `\t{Pattern: ${e.patternLit}, Re: regexp.MustCompile(${e.regexLit}), Context: ${e.context}, Output: ${e.output}, ` +
      `Levels: ${levelsLit(e.levels)}${featuresField(e)}},`
  )
}
lines.push('}')
lines.push('')

const next = lines.join('\n').split('\n').join(eol)
const current = existsSync(join(root, target)) ? readFileSync(join(root, target), 'utf8') : ''

// ---- 追加产出：站点级能力规则表 --------------------------------------------------
// 本段是纯追加逻辑：只读同一份内置资源的另一个数组，不触碰上面的 modelCatalogRules
// 生成路径，因此 model_catalog_gen.go 的字节结果保持不变。
//
// providerSiteRules 的每条规则绑定 baseUrlMatch（正则）+ 可选 apiTypeMatch，匹配语义
// 与 modelRules 一样是「顺序扫描 + 逐字段覆盖」，但多两道门：模型名、baseUrl、apiType
// 三个正则都命中才算命中。实测自 ZCode 内部分配器 ModelConfigRules.resolve：
//   sC=(e,t,n=false)=>new RegExp(`^(?:${e})$`, n?"i":void 0).test(t)
//   resolve: sC(o.modelMatch, api.modelId, true) && (apiTypeMatch 长度/命中) && (baseUrl 命中) → overlay
// 即三个正则都是全串锚定；模型名大小写不敏感，baseUrl / apiType 大小写敏感；
// baseUrl 先经 normalizeBaseURLForRuleMatch（去尾斜杠、保留 search/hash）再匹配。
const siteRules = parsed?.config?.modelConfigRules?.providerSiteRules
if (!Array.isArray(siteRules)) {
  console.error('内置资源缺少 config.modelConfigRules.providerSiteRules 数组，无法抽取站点规则表。')
  process.exit(2)
}

let siteBacktickFallback = 0
let siteSkipped = 0
let siteFeaturesCount = 0
// 反引号原始串；实测内置资源中不含反引号，若出现则退回双引号字面量。
const siteLit = (s) => {
  if (s.includes('`')) {
    siteBacktickFallback++
    return JSON.stringify(s)
  }
  return '`' + s + '`'
}
// RE2（Go 的 regexp 包）不支持的构造：前瞻/后顾、命名分组、原子组、条件组、反向引用。
// 内置资源目前一个都没有；一旦出现就报错退出，避免生成的 regexp.MustCompile 在 init 阶段 panic。
const RE2_UNSUPPORTED = /\(\?[=!]|\(\?<|\(\?P|\(\?>|\(\?\(|\\[1-9]/

const siteEntries = []
for (const rule of siteRules) {
  const model = typeof rule?.modelMatch === 'string' ? rule.modelMatch : ''
  const base = typeof rule?.baseUrlMatch === 'string' ? rule.baseUrlMatch : ''
  const api = typeof rule?.apiTypeMatch === 'string' ? rule.apiTypeMatch : ''
  // 缺匹配键的规则对 APM 没有意义（ZCode 里 modelMatch/baseUrlMatch 是必填的）。
  if (model === '' || base === '') {
    siteSkipped++
    continue
  }
  const modelRe = `(?i)^(?:${model})$`
  const baseRe = `^(?:${base})$`
  const apiRe = api === '' ? null : `^(?:${api})$`
  // 校验用原始 pattern（(?i) 是 Go 侧的写法，JS 不认）：既要能被 JS 编译（ZCode 自己
  // 也用它做 schema 校验），也要不含 RE2 不支持的构造。
  const checks = [
    ['modelMatch', model],
    ['baseUrlMatch', base],
    ['apiTypeMatch', api],
  ]
  let bad = null
  for (const [label, raw] of checks) {
    if (raw === '') continue
    if (RE2_UNSUPPORTED.test(raw)) {
      bad = `${label} 含 Go regexp(RE2) 不支持的构造：${raw}`
      break
    }
    try {
      new RegExp(`^(?:${raw})$`)
    } catch (e) {
      bad = `${label} 无法编译（${String(e && e.message ? e.message : e)}）：${raw}`
      break
    }
  }
  if (bad) {
    console.error(`providerSiteRules 第 ${siteEntries.length + siteSkipped} 条规则的 ${bad}`)
    process.exit(2)
  }
  const context = intOf(rule?.config?.properties?.contextWindow)
  const output = intOf(rule?.config?.optionSpecs?.maxOutputTokens?.max)
  const rawLevels = rule?.config?.optionSpecs?.reasoningLevel?.values
  const levels = Array.isArray(rawLevels) ? rawLevels.filter((s) => typeof s === 'string') : null
  const features = extractFeatures(rule?.config?.properties)
  if (features !== '') siteFeaturesCount++
  // 生成物自检：与 modelRules 同一口径（providerSiteRules 里整段缺 inputFormat 的条目更多，
  // 且它们现在排在家族正则之前，收窄输入的危害更大）。
  if (!checkMissingInputFormat('providerSiteRules', rule, features)) validationFailures++
  if (!checkEncodingShape('providerSiteRules', features)) validationFailures++
  siteEntries.push({ modelRe, baseRe, apiRe, context, output, levels, features })
}

// 生成物自检闸门：任何一条不过就直接退出，**不写出任何文件**（宁可保留旧生成物，也不写出
// 会把输入面收窄的错误表）。
if (validationFailures > 0) {
  console.error(`生成物自检失败：共 ${validationFailures} 处（详见上方报错），未写出任何文件。`)
  process.exit(2)
}
if (!checkOnly) {
  console.log(`输入面自检通过：${inputFormatMissingCount} 条规则整段没有 inputFormat（这一层不参与输入面），其输入段均为空。`)
}

const siteEol = existsSync(join(root, siteTarget)) ? eolOf(readFileSync(join(root, siteTarget), 'utf8')) : '\r\n'
const siteLines = [
  '// Code generated by scripts/sync-model-catalog.mjs from zcode-builtin.json. DO NOT EDIT.',
  '',
  'package core',
  '',
  'import "regexp"',
  '',
  '// 由 scripts/sync-model-catalog.mjs 抽取 providerSiteRules，源文件：',
  `//   ${source}`,
  `// 源 revision: ${revision}，规则条数: ${siteEntries.length}` +
    (siteSkipped > 0 ? `（另有 ${siteSkipped} 条缺少 modelMatch/baseUrlMatch 被跳过）` : ''),
  '//',
  '// 字段语义：',
  '//   Model / BaseURL / APIType 三个正则必须同时命中才算命中；APIType 为 nil 表示不限定 apiType。',
  '//   Context / Output 为 0 表示该规则未给出该字段；Levels 为 nil 表示未给出。',
  '//   Levels 原样保留 ZCode 的拼写，规范化交给运行时 normalizeCapabilityLevels。',
  '//',
  '// 匹配语义（实测自 ZCode 内部分配器 ModelConfigRules.resolve，其匹配函数为',
  "// `new RegExp('^(?:' + pattern + ')$', 大小写敏感标志).test(subject)`）：",
  '//   1. 三个正则都是**全串锚定** `^(?:...)$`（不是 unanchored 的 RegExp.test）；',
  '//      模型名额外大小写不敏感（实测传了 "i"），baseUrl 与 apiType 大小写敏感。',
  '//   2. baseUrl 匹配前先规范化（解析为绝对 URL、去掉结尾 `/`、保留 search/hash 尾缀）；',
  '//      baseUrl 为空或不是合法绝对 URL 时，任何站点规则都不适用（直接返回零值）。',
  '//   3. APIType 非 nil 时，调用方必须给出非空 apiType 且命中；apiType 为空即跳过该规则。',
  '//   4. 全表顺序扫描 + 逐字段覆盖：后命中规则里出现过的字段覆盖先前的值，未出现的字段保持不变。',
  '//      命中至少一条时 Source 记为 "site"。',
  '//',
  '// 文件头故意不写抽取日期，以保证 --check 跨天幂等（见脚本内注释）。',
  '',
  'var modelSiteRules = []siteRule{',
]
for (const e of siteEntries) {
  siteLines.push(
    `\t{Model: regexp.MustCompile(${siteLit(e.modelRe)}), BaseURL: regexp.MustCompile(${siteLit(e.baseRe)}), ` +
      `APIType: ${e.apiRe === null ? 'nil' : `regexp.MustCompile(${siteLit(e.apiRe)})`}, ` +
      `Context: ${e.context}, Output: ${e.output}, Levels: ${levelsLit(e.levels)}${featuresField(e)}},`
  )
}
siteLines.push('}')
siteLines.push('')
const siteNext = siteLines.join('\n').split('\n').join(siteEol)
const siteCurrent = existsSync(join(root, siteTarget)) ? readFileSync(join(root, siteTarget), 'utf8') : ''

const lineCount = (s) => (s === '' ? 0 : s.split(/\r\n|\n/).length)

if (checkOnly) {
  let ok = true
  if (current === next) {
    console.log(`模型能力目录已同步（${entries.length} 条，源 revision ${revision}，其中 ${featuresCount} 条带能力位）`)
  } else {
    ok = false
    console.error(`${target} 与内置资源不一致，请运行 node scripts/sync-model-catalog.mjs：`)
    console.error(`  - 期望 ${lineCount(next)} 行，磁盘 ${lineCount(current)} 行（差 ${lineCount(next) - lineCount(current)}）`)
  }
  if (siteCurrent === siteNext) {
    console.log(`站点能力规则表已同步（${siteEntries.length} 条，源 revision ${revision}，其中 ${siteFeaturesCount} 条带能力位）`)
  } else {
    ok = false
    console.error(`${siteTarget} 与内置资源不一致，请运行 node scripts/sync-model-catalog.mjs：`)
    console.error(`  - 期望 ${lineCount(siteNext)} 行，磁盘 ${lineCount(siteCurrent)} 行（差 ${lineCount(siteNext) - lineCount(siteCurrent)}）`)
  }
  process.exit(ok ? 0 : 1)
}

if (current === next) {
  console.log(`模型能力目录已一致，无需改动（${entries.length} 条，源 revision ${revision}，其中 ${featuresCount} 条带能力位）`)
} else {
  writeFileSync(join(root, target), next)
  console.log(`已生成 ${target}：${entries.length} 条规则，源 revision ${revision}（其中 ${featuresCount} 条带能力位）`)
}
if (backtickFallback > 0) {
  console.warn(`注意：${backtickFallback} 条 Pattern 含反引号，已改用双引号字面量。`)
}
if (siteCurrent === siteNext) {
  console.log(`站点能力规则表已一致，无需改动（${siteEntries.length} 条，源 revision ${revision}，其中 ${siteFeaturesCount} 条带能力位）`)
} else {
  writeFileSync(join(root, siteTarget), siteNext)
  console.log(`已生成 ${siteTarget}：${siteEntries.length} 条规则，源 revision ${revision}（其中 ${siteFeaturesCount} 条带能力位）`)
}
if (siteBacktickFallback > 0) {
  console.warn(`注意：${siteBacktickFallback} 条站点规则正则含反引号，已改用双引号字面量。`)
}
if (siteSkipped > 0) {
  console.warn(`注意：${siteSkipped} 条 providerSiteRules 缺少 modelMatch/baseUrlMatch，已跳过。`)
}
