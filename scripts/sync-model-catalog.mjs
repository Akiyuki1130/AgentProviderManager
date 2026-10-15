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
// 多记一个 '0' 只会让生成物变长而没有任何消费方。
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

// extractFeatures 把一条规则 config.properties 里的能力位编成紧凑串。
// 无任何已知为真的位时返回空串——**只有真的有能力位才写非空串**，这样生成物的每一行
// 要么带真实信息、要么保持最小的空串，跨次重生成不会产生噪音 diff。
//
// 有意不抽：requiresMfjsToolSchema、optionSpecs.*.map、config.enabled（见文件头说明）。
function extractFeatures(properties) {
  const props = properties && typeof properties === 'object' ? properties : {}
  const inputFmt = props.inputFormat && typeof props.inputFormat === 'object' ? props.inputFormat : {}
  const outputFmt = props.outputFormat && typeof props.outputFormat === 'object' ? props.outputFormat : {}
  const words = (fields, source) => fields.filter(([, key]) => source[key] === true).map(([word]) => word)
  const input = words(INPUT_MODALITY_FIELDS, inputFmt)
  const output = words(OUTPUT_MODALITY_FIELDS, outputFmt)
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
  siteEntries.push({ modelRe, baseRe, apiRe, context, output, levels, features })
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
