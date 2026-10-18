#!/usr/bin/env node
// 能力目录同步工具：把两份离线/快照数据源合并成
//   internal/core/capability_catalog_gen.go         —— 按小写精确模型 id 查 Context/Output/推理档位
//   internal/core/capability_features_gen.go        —— 按小写精确模型 id 查「能力位」（模态 / 工具调用 /
//                                                      结构化输出 / 联网搜索 / 中段 system / 附件 / 温度）
//
//   node scripts/sync-ability-catalog.mjs                  就地生成（完全离线）
//   node scripts/sync-ability-catalog.mjs --check          只校验；不一致时非零码退出
//   node scripts/sync-ability-catalog.mjs --update-snapshot 联网刷新 models.dev 快照后生成
//   node scripts/sync-ability-catalog.mjs --verify         用 OpenRouter 模型列表交叉校验生成表
//   node scripts/sync-ability-catalog.mjs --help           显示用法
//
// 数据来源：
//   1. DSH 桌面版自带的 pi-ai 离线目录（app.asar 内的
//      dsh/node_modules/@earendil-works/pi-ai/dist/providers/data/，41 个 provider json
//      + .manifest.json）。每个文件都按 manifest 里的 SHA-256 逐字节校验，校验失败立即报错退出。
//      可用环境变量 APM_DSH_ASAR 覆盖 app.asar 路径。
//   2. models.dev 的 api.json（MIT）。**默认只读仓库内的裁剪快照**
//      internal/core/data/models-dev-snapshot.json，因此生成与 --check 完全离线、跨天幂等。
//      只有 --update-snapshot 才会联网刷新快照。
//
// --verify 是**只读交叉校验**，不并入生成表：https://openrouter.ai/api/v1/models 的 id 带
// `vendor/` 前缀，且同一个模型对不同 provider 变体给不同的 max_completion_tokens，直接并入
// 会污染表（这也是当初只把 pi-ai 与 models.dev 作为生成来源的原因）。它只把 OpenRouter 的
// context_length / top_provider.max_completion_tokens 与我们的表逐条对比，打印不一致清单与
// 计数，**不写任何文件**（除 stdout），也不影响 --check / 生成模式的离线与幂等特性。
//
// 本脚本不读取任何用户目录下的配置或凭据，只读上述只读数据源。
import { readFileSync, writeFileSync, existsSync, mkdirSync, openSync, readSync, closeSync } from 'node:fs'
import { createHash } from 'node:crypto'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const argv = process.argv.slice(2)
const checkOnly = argv.includes('--check')
const updateSnapshot = argv.includes('--update-snapshot')
const verifyOnly = argv.includes('--verify')

const USAGE = [
  '能力目录同步工具（internal/core/capability_catalog_gen.go、internal/core/capability_features_gen.go）',
  '',
  '用法：',
  '  node scripts/sync-ability-catalog.mjs                   就地生成（完全离线）',
  '  node scripts/sync-ability-catalog.mjs --check           只校验；不一致时非零码退出（完全离线、跨天幂等）',
  '  node scripts/sync-ability-catalog.mjs --update-snapshot 联网刷新 models.dev 快照后生成',
  '  node scripts/sync-ability-catalog.mjs --verify          用 OpenRouter 模型列表交叉校验生成表（只读、只打印）',
  '  node scripts/sync-ability-catalog.mjs --help            显示本说明',
  '',
  '环境变量：',
  '  APM_DSH_ASAR  覆盖 DSH app.asar 路径（默认 C:\\AppData\\DSH Desktop\\resources\\app.asar）',
  '',
  '说明：',
  '  --verify 只做交叉校验，不并入生成表、不写任何文件；生成与 --check 永不需要网络。',
].join('\n')

if (argv.includes('--help') || argv.includes('-h')) {
  console.log(USAGE)
  process.exit(0)
}
if (checkOnly && (updateSnapshot || verifyOnly)) {
  console.error('--check 不能与 --update-snapshot / --verify 同时使用（--check 必须完全离线）。')
  console.error(USAGE)
  process.exit(2)
}

// ---- 路径与常量 ----------------------------------------------------------------

const DEFAULT_ASAR = 'C:\\AppData\\DSH Desktop\\resources\\app.asar'
const asarPath = process.env.APM_DSH_ASAR || DEFAULT_ASAR

// asar 里 pi-ai 离线目录的路径分段。
const PI_AI_SEGMENTS = ['dsh', 'node_modules', '@earendil-works', 'pi-ai', 'dist', 'providers', 'data']

const SNAPSHOT_REL = 'internal/core/data/models-dev-snapshot.json'
const TARGET_REL = 'internal/core/capability_catalog_gen.go'
// 并列产物：能力位表。与上面那张 `ctx|out|levels` 表**彼此独立**（互不解析对方），
// 因此旧表「恰好三段、fail-closed」的解析契约一点没动。
const FEATURES_TARGET_REL = 'internal/core/capability_features_gen.go'

const MODELS_DEV_URL = 'https://models.dev/api.json'
// OpenRouter 公开免鉴权的模型列表，**只用于 --verify 交叉校验**，绝不进入生成表。
const OPENROUTER_MODELS_URL = 'https://openrouter.ai/api/v1/models'
// 直连失败时尝试的本机代理（--update-snapshot 与 --verify 共用）。
const LOCAL_PROXY = 'http://127.0.0.1:7897'

// 原厂供应商：模型能力的真实出处。同名 id 冲突时，原厂条目优先于聚合器/托管商/中转商。
// 这张白名单刻意只收「自己造模型」的厂商；其余一律视为聚合器（openrouter、vercel、
// opencode、bedrock、huggingface、各种 token-plan 中转……）。
const ORIGINAL_VENDORS = new Set([
  'anthropic', 'openai', 'google', 'deepseek', 'meta', 'mistral', 'minimax',
  'moonshotai', 'xai', 'zai', 'zhipuai', 'alibaba', 'stepfun', 'cohere', 'ai21', 'xiaomi',
])

// 档位名原样透传（不在这里做规范化）：运行时由 normalizeCapabilityLevels 负责。
const mdInt = (v) => (typeof v === 'number' && Number.isFinite(v) && v > 0 ? Math.trunc(v) : 0)

// ---- 能力位编码 ----------------------------------------------------------------
//
// 编码是 `<input>|<output>|<flags>` 三段，与 internal/core/capability_features.go 的
// parseFeatureEncoding 共享契约（两侧任何改动都必须同步）：
//   input / output  逗号分隔的模态词，规范序见下；未知写空段；
//   flags           **恰好 6 个字符**，固定序 tjwmaT，每字符 '1'（已知为真）/ '-'（未知）。
// 只写「已知为真」：'0' 与 '-' 在解析上等价（都表示未知、不落盘），所以本脚本从不产出 '0'。
const FEATURE_FLAG_ORDER = 'tjwmaT'
// 与 internal/core/model_features.go 的 inputModalityOrder / outputModalityOrder 同序；
// 也是解析端 normalizeModalities 会保留的词表。词表外的取值一律丢弃（宁可少不可错）。
const INPUT_MODALITY_ORDER = ['text', 'image', 'video', 'audio', 'pdf']
const OUTPUT_MODALITY_ORDER = ['text', 'audio', 'image', 'video', 'pdf']

// pi-ai `compat` 的**显式白名单**：只有这三个键进中立层。pi-ai 的 compact 白名单有 27 个键，
// 其余（supportsDeveloperRole / supportsOpenAIGrammarTools / thinkingFormat / chatTemplateArgs …）
// 要么与中立层的语义不等价、要么压根不是能力位（是请求体细节），映射它们就是编造。
// 特别地 supportsMidConvoSystemMessages 是 pi-ai 的 catalog-only 键（DSH 的配置层
// assertOfferedCompatFields 会拒绝用户写入），但它确实描述模型能力，因此照收。
const PI_COMPAT_FLAG_MAP = [
  ['supportsStrictMode', 'j'], // 结构化输出（DSH 在 constrained-sampling 里消费）
  ['supportsTemperature', 'T'], // 支持温度参数
  ['supportsMidConvoSystemMessages', 'm'], // 对话中途 system
]

// models.dev 布尔位 → 中立层 flag 字符（快照字段名见下方 buildSnapshot）。
const MD_FLAG_MAP = [
  ['tool_call', 't'],
  ['structured_output', 'j'],
  ['attachment', 'a'],
  ['temperature', 'T'],
]

// pi-ai 目录里 MODALITIES 只有 {text, image}，但没有理由在代码里写死这个上限：词表校验本来
// 就会挡掉未识别的词，写足五词后上游一旦放宽也不用改这里。
const normalizeModalities = (list, order) => {
  const set = new Set()
  for (const raw of Array.isArray(list) ? list : []) {
    if (typeof raw !== 'string') continue
    const word = raw.trim().toLowerCase()
    if (order.includes(word)) set.add(word)
  }
  return order.filter((w) => set.has(w))
}

// emptyFeatures / encodeFeatures：能力位的内部表示是 {input, output, bits}，bits 里只放
// **已知为真**的 flag 字符，因此「有无能力信息」就是「集合是否为空」。
const emptyFeatures = () => ({ input: [], output: [], bits: new Set() })
const isEmptyFeatures = (f) => f.input.length === 0 && f.output.length === 0 && f.bits.size === 0

function encodeFeatures(f) {
  const flags = FEATURE_FLAG_ORDER.split('')
    .map((ch) => (f.bits.has(ch) ? '1' : '-'))
    .join('')
  return `${f.input.join(',')}|${f.output.join(',')}|${flags}`
}

// unionModalities 按规范序取两个模态集合的并集（去重 + 排序）。
const unionModalities = (a, b, order) => {
  const set = new Set([...a, ...b])
  return order.filter((w) => set.has(w))
}

// unionFeatures 合并两层的能力位，两条口径**都取正向的并集**（主会话 2026-10-15 裁决）：
//
//   模态数组 → **并集**。模态是「已知支持」的正向集合，缺项只代表「这一层没声明」，不代表
//   否定；pi-ai 的 MODALITIES 词表本来就只有 {text, image}，同一个 id 在 models.dev 侧常给
//   出 video / pdf / audio，若让 pi-ai 的短集合「先占位」就会永久丢掉这些真值（实测 363 个
//   两层都有的 id 里有 211 个会被压小）。因此这里按词并集，而不是「已有真值不被覆盖」。
//   布尔位 → **或**。两层都只会带来真值（`'0'`/缺键在编码上都是「未知」），任一来源为真
//   即为真，等价于「只补未知的」。
//
// 注意这条并集口径只用于**生成期**（把两份目录数据源合成一张表）。运行时的分层合并
// （capability.go 的 mergeFeatures：provider > override > zcode > site > catalog > preset，
// 前层胜出、后层填空缺）语义完全不变——那里是「谁的 ctx/能力更权威」，与这里「两份社区目录
// 互相补全」不是一回事。
function unionFeatures(dst, src) {
  dst.input = unionModalities(dst.input, src.input, INPUT_MODALITY_ORDER)
  dst.output = unionModalities(dst.output, src.output, OUTPUT_MODALITY_ORDER)
  for (const ch of src.bits) dst.bits.add(ch)
}

// featuresOfModel 把 models.dev 的一条模型记录读成能力位：`modalities{input,output}` 取
// 白名单内的模态词，`tool_call` / `structured_output` / `attachment` / `temperature` 严格
// `=== true` 才算已知为真。**live api.json 与仓库快照的字段同名**，所以同一个函数两处复用；
// 对缺字段完全容忍——旧快照的条目只有 context/output/reasoning/levels 四键，读出来就是
// 「能力位全空」，绝不报错（能力位表因此只剩 pi-ai 一半来源，这是可接受、且必须不崩的降级）。
// 这里不读 reasoning / reasoning_options：它们是档位信息，走既有的 levels 编码。
function featuresOfModel(m) {
  const f = emptyFeatures()
  if (!m || typeof m !== 'object') return f
  const mod = m.modalities && typeof m.modalities === 'object' ? m.modalities : {}
  f.input = normalizeModalities(mod.input, INPUT_MODALITY_ORDER)
  f.output = normalizeModalities(mod.output, OUTPUT_MODALITY_ORDER)
  for (const [key, flag] of MD_FLAG_MAP) {
    if (m[key] === true) f.bits.add(flag)
  }
  return f
}

// ---- asar 读取 ----------------------------------------------------------------

// asar 头部：前 16 字节里，[12..16) 是 header JSON 的字节长度，JSON 从 16 开始，
// 文件数据从 16 + headerSize 开始（实测等于 8 + UInt32LE(4)，两者必须一致）。
function readAsarDir(asarPath, segments) {
  if (!existsSync(asarPath)) {
    console.error(`未找到 DSH 应用包：${asarPath}`)
    console.error('请确认 DSH Desktop 已安装，或用环境变量 APM_DSH_ASAR 指定 app.asar 路径。')
    process.exit(2)
  }
  const fd = openSync(asarPath, 'r')
  // 整段读取只开一次文件；readAt 保证短读时补齐。
  const readAt = (pos, len) => {
    const buf = Buffer.alloc(len)
    let got = 0
    while (got < len) {
      const n = readSync(fd, buf, got, len - got, pos + got)
      if (n <= 0) break
      got += n
    }
    return buf
  }
  try {
    const head = readAt(0, 16)
    const headerSize = head.readUInt32LE(12)
    const dataBaseByHead = 16 + headerSize
    const dataBaseBySize = 8 + head.readUInt32LE(4)
    if (headerSize <= 0 || dataBaseByHead !== dataBaseBySize) {
      console.error(`asar 头部偏移自检失败（headerSize=${headerSize}，两种算法不一致），请复核文件格式。`)
      process.exit(2)
    }
    const header = JSON.parse(readAt(16, headerSize).toString('utf8'))
    let node = header
    for (const seg of segments) {
      node = node && node.files ? node.files[seg] : undefined
      if (!node) {
        console.error(`asar 内缺少目录：${segments.join('/')}（缺失于 ${seg}）`)
        process.exit(2)
      }
    }
    const dir = {}
    for (const [name, meta] of Object.entries(node.files || {})) {
      if (meta.files) continue // 只取文件（不递归子目录）
      const bytes = readAt(dataBaseByHead + Number(meta.offset), meta.size)
      dir[name] = { size: meta.size, bytes, sha256: createHash('sha256').update(bytes).digest('hex') }
    }
    return dir
  } finally {
    closeSync(fd)
  }
}

// 用 manifest 的 SHA-256 逐文件校验：任何一处不一致都说明 DSH 内的 pi-ai 目录被改动或
// 抽取偏移算错，此时必须报错退出而不是产出可疑数据。
function verifyManifest(dir) {
  const manifestName = '.manifest.json'
  const raw = dir[manifestName]
  if (!raw) {
    console.error('pi-ai data 目录缺少 .manifest.json，无法校验来源完整性。')
    process.exit(2)
  }
  const manifest = JSON.parse(raw.bytes.toString('utf8'))
  const files = manifest.files || {}
  let checked = 0
  for (const [name, want] of Object.entries(files)) {
    const got = dir[name]
    if (!got) {
      console.error(`manifest 声明了 ${name}，但 asar 内不存在。`)
      process.exit(2)
    }
    if (got.sha256 !== String(want).toLowerCase()) {
      console.error(`pi-ai 文件校验失败：${name}`)
      console.error(`  期望 ${want}`)
      console.error(`  实得 ${got.sha256}`)
      process.exit(2)
    }
    checked++
  }
  return { manifest, checked }
}

// ---- pi-ai 抽取 --------------------------------------------------------------

// 每条模型的能力来源信息：
//   ctx/out 为 0 表示未知；levels 为原样档位串（可能为空数组）；
//   features 是能力位集合（input 模态 + compat 白名单布尔位；pi-ai 不表达输出模态）。
// pi-ai 目录的模型键现在带「模型类型」前缀（`chat:` / `image:` / `classifier:`），且每条记录
// 自带显式的裸 `id`：
//   `"chat:amazon.nova-2-lite-v1:0": { "id": "amazon.nova-2-lite-v1:0", "type": "chat", … }`
// 本表的索引必须是**用户配置与 API 响应里出现的模型 id**——`chat:` 只是目录内部的键前缀，
// 所以取记录自己的 `id`；旧格式（键即裸 id、无 `type` 字段）照旧走键。注意 `id` 自身可能含
// 冒号（`amazon.nova-2-lite-v1:0`），因此「键前缀」只在字形恰好是已知类型时才当类型看。
// 非对话模型（image / classifier）不进本表：它们的 ctx 与档位语义跟对话模型不是一回事，
// 而旧数据里只有对话模型，收进来等于凭空扩面。
const PI_KEY_TYPES = new Set(['chat', 'image', 'classifier'])

function extractPiAI(dir) {
  const out = []
  let skippedNoProvider = 0
  let skippedNonChat = 0
  const names = Object.keys(dir)
    .filter((n) => n.endsWith('.json') && n !== '.manifest.json')
    .sort()
  for (const name of names) {
    let parsed
    try {
      parsed = JSON.parse(dir[name].bytes.toString('utf8'))
    } catch (e) {
      console.error(`解析 pi-ai provider 文件失败：${name}（${String(e && e.message)}）`)
      process.exit(2)
    }
    for (const apiKey of Object.keys(parsed)) {
      const models = parsed[apiKey]
      if (!models || typeof models !== 'object') continue
      for (const [rawKey, m] of Object.entries(models)) {
        if (!m || typeof m !== 'object') continue
        const colon = rawKey.indexOf(':')
        const keyType = colon > 0 ? rawKey.slice(0, colon).toLowerCase() : ''
        const prefixed = PI_KEY_TYPES.has(keyType)
        const declaredType =
          typeof m.type === 'string' && m.type !== '' ? m.type.toLowerCase() : prefixed ? keyType : ''
        if (declaredType && declaredType !== 'chat') {
          skippedNonChat++
          continue
        }
        const explicitID = typeof m.id === 'string' ? m.id.trim() : ''
        const modelID = explicitID !== '' ? explicitID : prefixed ? rawKey.slice(colon + 1) : rawKey
        if (modelID === '') continue
        const provider = String(m.provider || '').toLowerCase()
        if (!provider) skippedNoProvider++
        const ctx = mdInt(m.contextWindow)
        const out2 = mdInt(m.maxTokens)
        // 档位：thinkingLevelMap 里 value != null 的键；reasoning === false 直接视为无档位。
        let levels = []
        if (m.reasoning !== false && m.thinkingLevelMap && typeof m.thinkingLevelMap === 'object') {
          levels = Object.entries(m.thinkingLevelMap)
            .filter(([, v]) => v !== null && v !== undefined)
            .map(([k]) => k)
            .filter((s) => typeof s === 'string' && s !== '')
        }
        out.push({
          id: modelID.toLowerCase(),
          provider: provider || name.replace(/\.json$/, ''),
          ctx,
          out: out2,
          levels,
          features: extractPiFeatures(m),
          file: name,
        })
      }
    }
  }
  return { entries: out, skippedNoProvider, skippedNonChat }
}

// extractPiFeatures 从一条 pi-ai 模型记录抽能力位：`input[]` 是输入模态的**正向声明**，
// `compat` 只认显式白名单里的三个键。工具调用 / 联网搜索 / 附件 pi-ai 完全没有表达，
// 保持未知（'-'），不去猜。
function extractPiFeatures(m) {
  const f = emptyFeatures()
  f.input = normalizeModalities(m.input, INPUT_MODALITY_ORDER)
  const compat = m.compat && typeof m.compat === 'object' ? m.compat : {}
  for (const [key, flag] of PI_COMPAT_FLAG_MAP) {
    if (compat[key] === true) f.bits.add(flag)
  }
  return f
}

// ---- models.dev 快照 ----------------------------------------------------------

// 每 provider 的 models map：limit.context（缺则用 limit.input 兜底）、limit.output、
// reasoning 布尔，reasoning_options 里 type=="effort" 的 values 作为档位；
// 另加宽出的能力位：modalities{input,output}（只留词表内取值、按规范序）、
// tool_call / structured_output / attachment / temperature（**只在为 true 时写键**，
// 缺键 = 未知或假，与生成表「只写真」的口径一致，也让快照不至于为四个 false 膨胀）。
//
// 只保留「无命名空间前缀」的 id：带 `/` 的 id（anthropic/claude-opus-5、sao10k/l3-lunaris-8b）
// 是聚合器的命名空间镜像，原厂那份通常以裸 id 存在；保留它们会让表体积翻倍且几乎不会被命中。
function buildSnapshot(api) {
  const providers = {}
  let kept = 0
  let dropped = 0
  for (const pid of Object.keys(api).sort()) {
    const models = api[pid] && api[pid].models
    if (!models || typeof models !== 'object') continue
    const bucket = {}
    for (const rawID of Object.keys(models).sort()) {
      const id = rawID.toLowerCase()
      if (id.includes('/')) {
        dropped++
        continue
      }
      const m = models[rawID] || {}
      const limit = m.limit || {}
      let context = mdInt(limit.context)
      if (context === 0) context = mdInt(limit.input)
      const output = mdInt(limit.output)
      const reasoning = m.reasoning === true
      const levels = []
      if (Array.isArray(m.reasoning_options)) {
        for (const opt of m.reasoning_options) {
          if (!opt || opt.type !== 'effort' || !Array.isArray(opt.values)) continue
          for (const v of opt.values) if (typeof v === 'string' && v !== '') levels.push(v)
        }
      }
      // 加宽后的能力位：模态取全词表（models.dev 侧确实给出五词），布尔位只记 true。
      const f = featuresOfModel(m)
      // 全无信息的条目没有任何价值，不入快照。判据从「ctx/out/reasoning/levels 皆空」
      // 扩到「能力位也全空」——快照现在同时服务两张表，只有模态/附件位的模型对能力位表
      // 仍然有用，不该再被旧判据丢掉。
      if (context === 0 && output === 0 && !reasoning && levels.length === 0 && isEmptyFeatures(f)) {
        dropped++
        continue
      }
      const entry = { context, output, reasoning, levels }
      if (f.input.length > 0 || f.output.length > 0) {
        entry.modalities = { input: f.input, output: f.output }
      }
      for (const [key, flag] of MD_FLAG_MAP) if (f.bits.has(flag)) entry[key] = true
      bucket[id] = entry
      kept++
    }
    if (Object.keys(bucket).length > 0) providers[pid] = bucket
  }
  return { providers, kept, dropped }
}

// 直连优先、失败再走本机代理的取文本工具（--update-snapshot 与 --verify 共用）。
async function fetchTextWithFallback(url) {
  const errors = []
  try {
    return { text: await fetchText(url, null), via: 'direct' }
  } catch (e) {
    errors.push(`直连失败：${String(e && e.message ? e.message : e)}`)
  }
  try {
    return { text: await fetchText(url, LOCAL_PROXY), via: `proxy ${LOCAL_PROXY}` }
  } catch (e) {
    errors.push(`代理失败：${String(e && e.message ? e.message : e)}`)
  }
  return { text: null, via: '', errors }
}

async function fetchModelsDev() {
  return fetchTextWithFallback(MODELS_DEV_URL)
}

async function fetchText(url, proxy) {
  if (!proxy) {
    const res = await fetch(url, { signal: AbortSignal.timeout(60000) })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    return await res.text()
  }
  // Node 的 fetch 不认 http 代理，这里手写 CONNECT 隧道 + 明文 HTTP/1.1 请求。
  const { request } = await import('node:http')
  const { connect } = await import('node:tls')
  const u = new URL(url)
  const p = new URL(proxy)
  const socket = await new Promise((resolve, reject) => {
    const req = request({
      host: p.hostname,
      port: Number(p.port || 80),
      method: 'CONNECT',
      path: `${u.hostname}:443`,
      timeout: 30000,
    })
    req.on('connect', (res, sock) => (res.statusCode === 200 ? resolve(sock) : reject(new Error(`CONNECT ${res.statusCode}`))))
    req.on('timeout', () => reject(new Error('CONNECT 超时')))
    req.on('error', reject)
    req.end()
  })
  const tls = connect({ socket, servername: u.hostname })
  const chunks = []
  await new Promise((resolve, reject) => {
    tls.on('secureConnect', () => {
      tls.write(
        `GET ${u.pathname}${u.search} HTTP/1.1\r\nHost: ${u.hostname}\r\n` +
          'User-Agent: apm-sync-ability-catalog\r\nAccept: application/json\r\n' +
          'Accept-Encoding: identity\r\nConnection: close\r\n\r\n'
      )
    })
    tls.on('data', (c) => chunks.push(c))
    tls.on('end', resolve)
    tls.on('error', reject)
  })
  // CONNECT 隧道之后拿到的是源站原样的 HTTP/1.1 响应，可能带 chunked 编码，
  // 因此这里按字节流解析（先按头部声明做 de-chunk / 截断），再统一转 UTF-8。
  const raw = Buffer.concat(chunks)
  const sep = raw.indexOf('\r\n\r\n')
  if (sep < 0) throw new Error('代理响应缺少头部结束符')
  const head = raw.slice(0, sep).toString('latin1')
  const status = Number((head.split('\r\n')[0].split(' ')[1] || '0'))
  if (status !== 200) throw new Error(`HTTP ${status}`)
  let body = raw.slice(sep + 4)
  if (/transfer-encoding:\s*chunked/i.test(head)) {
    body = dechunk(body)
  } else {
    const len = /content-length:\s*(\d+)/i.exec(head)
    if (len) body = body.slice(0, Number(len[1]))
  }
  return body.toString('utf8')
}

// dechunk 只解 HTTP/1.1 的分块编码：每块是「十六进制长度行 + CRLF + 数据 + CRLF」，
// 长度 0 表示结束；遇到畸形块就停止（返回已经解出的部分），由上层 JSON 解析去报错。
function dechunk(buf) {
  const out = []
  let pos = 0
  while (pos < buf.length) {
    const eol = buf.indexOf('\r\n', pos)
    if (eol < 0) break
    const size = parseInt(buf.slice(pos, eol).toString('latin1').split(';')[0].trim(), 16)
    if (!Number.isFinite(size) || size <= 0) break
    const start = eol + 2
    out.push(buf.slice(start, start + size))
    pos = start + size + 2
  }
  return Buffer.concat(out)
}

function snapshotMeta(source, fetchedAt, note) {
  return { source, fetchedAt, note }
}

// ---- 合并 --------------------------------------------------------------------

// 优先级 = 原厂(0) / 聚合器(1) × 2 + 数据源（pi-ai 0，models.dev 1）。
// 同一个 id 只保留优先级最高的一份；同优先级按扫描顺序先到先得（顺序是确定性的：
// pi-ai 按文件名+文件内顺序，models.dev 按 provider 名+模型名排序）。
function rankOf(provider, sourceTier) {
  return (ORIGINAL_VENDORS.has(String(provider || '').toLowerCase()) ? 0 : 1) * 2 + sourceTier
}

function merge(piEntries, mdProviders) {
  const chosen = new Map()
  const stats = { conflicts: 0, backtickFallback: 0, empty: 0, conflictsByKey: [] }

  const put = (id, ctx, out, levels, provider, sourceTier, origin) => {
    if (!id) return
    if (ctx === 0 && out === 0 && levels.length === 0) {
      stats.empty++
      return
    }
    const rank = rankOf(provider, sourceTier)
    const prev = chosen.get(id)
    if (prev) {
      if (prev.rank <= rank) {
        stats.conflicts++
        if (stats.conflictsByKey.length < 20) {
          stats.conflictsByKey.push(`${id}（保留 ${prev.provider}/${prev.origin}，丢弃 ${provider}/${origin}）`)
        }
        return
      }
      stats.conflicts++
    }
    chosen.set(id, { id, ctx, out, levels, provider: provider || 'unknown', origin, rank })
  }

  for (const e of piEntries) put(e.id, e.ctx, e.out, e.levels, e.provider, 0, e.file)
  for (const pid of Object.keys(mdProviders).sort()) {
    const models = mdProviders[pid]
    for (const id of Object.keys(models).sort()) {
      const m = models[id]
      put(id, mdInt(m.context), mdInt(m.output), Array.isArray(m.levels) ? m.levels : [], pid, 1, 'models.dev')
    }
  }
  return { chosen, stats }
}

// ---- 能力位合并 ----------------------------------------------------------------

// mergeFeatures 是**另一套口径**，与上面 merge() 的「原厂 vs 聚合器 + 数据源」排序无关：
// 能力位只按数据源分层——pi-ai 精确表在前、models.dev 在后，两层**合并为并集**
// （模态数组取词并集、布尔位取或，理由见 unionFeatures 的注释）。
//
// 为什么不复用 merge() 的原厂优先级？因为能力位关心的是「谁声明了这一个位」，而不是
// 「这个 id 该信谁的 ctx」——同一个 id 完全可能 ctx 取 models.dev 的原厂条目、勾选位取
// pi-ai 的精确条目（例如 pi-ai 说 supportsStrictMode: true 而 models.dev 说
// structured_output: false）。两张表各自独立、互不影响，也各自独立 --check。
//
// 只收录「至少有一位已知为真」的 id：一个 id 若两层都没有任何真值，就不落表。
//
// stats.unionGrew 记录「模态集合因并集而变大」的 id 数（含 models.dev 同一 id 跨 provider
// 的补全），供人工核对并集口径的实际增量。
function mergeFeatures(piEntries, mdProviders) {
  const table = new Map()
  const grew = new Set()
  const stats = { piCandidates: 0, mdCandidates: 0, skippedEmpty: 0, piOnly: 0, mdOnly: 0, both: 0, unionGrew: 0 }

  const put = (id, feat, tier) => {
    if (!id) return
    if (isEmptyFeatures(feat)) {
      stats.skippedEmpty++
      return
    }
    if (tier === 'pi') stats.piCandidates++
    else stats.mdCandidates++
    const cur = table.get(id)
    if (!cur) {
      table.set(id, {
        id,
        input: feat.input.slice(),
        output: feat.output.slice(),
        bits: new Set(feat.bits),
        tiers: new Set([tier]),
      })
      return
    }
    const before = cur.input.length + cur.output.length
    unionFeatures(cur, feat)
    if (cur.input.length + cur.output.length > before) grew.add(id)
    cur.tiers.add(tier)
  }

  for (const e of piEntries) put(e.id, e.features, 'pi')
  for (const pid of Object.keys(mdProviders).sort()) {
    const models = mdProviders[pid]
    for (const id of Object.keys(models).sort()) {
      put(id, featuresOfModel(models[id]), 'md')
    }
  }
  for (const entry of table.values()) {
    const pi = entry.tiers.has('pi')
    const md = entry.tiers.has('md')
    if (pi && md) stats.both++
    else if (pi) stats.piOnly++
    else stats.mdOnly++
  }
  stats.unionGrew = grew.size
  return { table, stats }
}

// 按位统计「有多少个 id 的该位为真」，供生成物文件头与人工核对覆盖率：
// 逐位计数（不是逐 id），因此各数之和会大于表条数。
function featureBitTally(table) {
  const tally = { inputText: 0, inputImage: 0, withOutput: 0 }
  for (const ch of FEATURE_FLAG_ORDER) tally[ch] = 0
  for (const e of table.values()) {
    for (const ch of FEATURE_FLAG_ORDER) if (e.bits.has(ch)) tally[ch]++
    if (e.input.includes('text')) tally.inputText++
    if (e.input.includes('image')) tally.inputImage++
    if (e.output.length > 0) tally.withOutput++
  }
  return tally
}

// ---- OpenRouter 交叉校验（--verify，只读） -------------------------------------

// 与 internal/core/capability.go 的 lookupCatalogTolerant 同口径的容错查询：
// 先按小写精确 id 找；未命中且 id 含 "/" 时，再取最后一个 "/" 之后的一段找。
// fail-closed：只在第二次真的命中时才采用，绝不猜；两次都未命中返回 null。
function tolerantLookup(index, rawID) {
  const low = String(rawID == null ? '' : rawID).toLowerCase()
  if (low === '') return null
  if (index.has(low)) return { key: low, entry: index.get(low), via: 'exact' }
  const i = low.lastIndexOf('/')
  if (i >= 0) {
    const tail = low.slice(i + 1)
    if (tail !== '' && index.has(tail)) return { key: tail, entry: index.get(tail), via: 'tail' }
  }
  return null
}

// origin 为 'models.dev' 的是快照来源，其余是 pi-ai 的文件名。
function sourceLabel(entry) {
  return entry.origin === 'models.dev' ? `models.dev:${entry.provider}` : `pi-ai:${entry.origin}`
}

const VERIFY_PRINT_LIMIT = 200

// verifyAgainstOpenRouter 用 OpenRouter 的公开模型列表交叉校验我们的表。
// 它**只读**：不写文件、不改生成物、不影响生成与 --check 的离线幂等行为。
// 不一致判定：OpenRouter 给出 > 0 的数值且与表内值不同（表内 0 表示未知，会照报并标「未知」）。
// 归一后对不上的 id 计入「OpenRouter 独有 / 我们独有」，不算不一致。
async function verifyAgainstOpenRouter(chosen) {
  const res = await fetchTextWithFallback(OPENROUTER_MODELS_URL)
  if (!res.text) {
    console.error('拉取 OpenRouter 模型列表失败：')
    for (const e of res.errors || []) console.error(`  - ${e}`)
    process.exit(1)
  }
  let payload
  try {
    payload = JSON.parse(res.text)
  } catch (e) {
    console.error(`OpenRouter 响应不是合法 JSON（经由 ${res.via}）：${String(e && e.message ? e.message : e)}`)
    console.error(`  响应前 200 字节：${JSON.stringify(res.text.slice(0, 200))}`)
    process.exit(1)
  }
  const models = Array.isArray(payload.data) ? payload.data : []

  const matchedKeys = new Set()
  const rows = []
  let compared = 0
  let consistent = 0
  let mismatched = 0
  let ctxDiffs = 0
  let outDiffs = 0
  let orOnly = 0

  for (const m of models) {
    const orID = String((m && m.id) || '')
    if (orID === '') continue
    const hit = tolerantLookup(chosen, orID)
    if (!hit) {
      orOnly++
      continue
    }
    matchedKeys.add(hit.key)
    compared++
    const orCtx = mdInt(m.context_length)
    const orOut = mdInt(m.top_provider && m.top_provider.max_completion_tokens)
    const src = sourceLabel(hit.entry) + (hit.via === 'tail' ? `（表内 ${hit.key}）` : '')
    const show = (v) => (v > 0 ? String(v) : '0（未知）')
    let bad = false
    if (orCtx > 0 && orCtx !== hit.entry.ctx) {
      rows.push(`${orID} | context_length | ${orCtx} | ${show(hit.entry.ctx)} | ${src}`)
      ctxDiffs++
      bad = true
    }
    if (orOut > 0 && orOut !== hit.entry.out) {
      rows.push(`${orID} | max_completion_tokens | ${orOut} | ${show(hit.entry.out)} | ${src}`)
      outDiffs++
      bad = true
    }
    if (bad) mismatched++
    else consistent++
  }

  const ourOnly = chosen.size - matchedKeys.size
  console.log(`OpenRouter 交叉校验（只读，不并入生成表）：${OPENROUTER_MODELS_URL}（经由 ${res.via}）`)
  console.log('比对口径：两侧都做「小写 + 去命名空间前缀」归一（与 core.lookupCatalogTolerant 同口径）；')
  console.log('          OpenRouter 侧为 null/0 的字段不参与比对。')
  console.log('')
  console.log(`不一致清单（id | 字段 | OpenRouter 值 | 表内值 | 来源），共 ${rows.length} 行：`)
  for (const row of rows.slice(0, VERIFY_PRINT_LIMIT)) console.log(`  ${row}`)
  if (rows.length > VERIFY_PRINT_LIMIT) {
    console.log(`  ……（还有 ${rows.length - VERIFY_PRINT_LIMIT} 行未打印）`)
  }
  console.log('')
  console.log('汇总：')
  console.log(`  OpenRouter 模型数：${models.length}`)
  console.log(`  比对条数：${compared}`)
  console.log(`  一致：${consistent}`)
  console.log(`  不一致：${mismatched}（其中 context_length ${ctxDiffs} 行、max_completion_tokens ${outDiffs} 行）`)
  console.log(`  OpenRouter 独有（归一后表里没有）：${orOnly}`)
  console.log(`  我们独有（表里未被任何 OpenRouter id 命中）：${ourOnly}`)
}

// ---- Go 代码生成 --------------------------------------------------------------

// 反引号原始串；含反引号时退化为双引号字面量。
function lit(s, stats) {
  if (s.includes('`')) {
    stats.backtickFallback++
    return JSON.stringify(s)
  }
  return '`' + s + '`'
}

const eolOf = (raw) => (raw.includes('\r\n') ? '\r\n' : '\n')

function generate(chosen, facts, stats) {
  const ids = [...chosen.keys()].sort()
  const lines = [
    '// Code generated by scripts/sync-ability-catalog.mjs. DO NOT EDIT.',
    '',
    'package core',
    '',
    '// 由 scripts/sync-ability-catalog.mjs 生成，按「小写精确模型 id」索引。',
    '//',
    '// 数据来源（两份，均为只读）：',
    `//   1. DSH pi-ai 离线目录 ${facts.piDir}`,
    `//      manifest structureHash: ${facts.manifestHash}（manifest generatedAt ${facts.manifestGeneratedAt}）`,
    `//      已按 manifest 的 SHA-256 逐文件校验 ${facts.verifiedFiles} 个 provider 文件。`,
    '//      表索引取每条记录自己的 `id` 字段（目录里的键带 `chat:` / `image:` / `classifier:`',
    '//      类型前缀，那是目录内部的命名，配置与 API 响应里都没有），且只收 `chat` 类型的记录。',
    `//   2. models.dev ${facts.snapshotSource}`,
    `//      快照拉取时刻 ${facts.snapshotFetchedAt}；快照位于 ${SNAPSHOT_REL}`,
    `//      （快照是裁剪后的 {providerId: {modelId: {context,output,reasoning,levels}}}）。`,
    '//',
    `// 合并规则：同一 id 只保留一份，原厂供应商（${facts.originalVendors}）优先于`,
    '// 聚合器/托管商；同优先级时 pi-ai 优先于 models.dev。全部小写化后建表。',
    `// 本表共 ${ids.length} 条（pi-ai ${facts.piUnique} 条候选、models.dev ${facts.mdUnique} 条候选，`,
    `// 冲突丢弃 ${stats.conflicts} 条、因无任何信息跳过 ${stats.empty} 条；`,
    `// 最终条目的来源构成：pi-ai ${facts.piWinners} 条、models.dev ${ids.length - facts.piWinners} 条）。`,
    '//',
    '// 值编码为 `ctx|out|levels` 三段：',
    '//   ctx  / out  十进制整数，0 表示该字段未知；',
    '//   levels      逗号分隔的档位名，**原样保留来源拼写**（规范化交给运行时',
    '//               normalizeCapabilityLevels）；空串表示来源没有给出档位枚举',
    '//               （模型可能仍支持思考，但目录里没有可枚举的档位集合）。',
    '//',
    '// 文件头故意不写抽取日期：--check 做整文件字节比较，写上「今天」会让跨天的校验',
    '// 无故失败。header 里的 revision/摘要/SHA 校验结果都只随真实数据变化；models.dev',
    '// 的抓取时刻来自快照文件本身，只有显式 --update-snapshot 才会改变。',
    '',
    'var catalogEntries = map[string]string{',
  ]
  for (const id of ids) {
    const e = chosen.get(id)
    const enc = `${e.ctx}|${e.out}|${e.levels.join(',')}`
    lines.push(`\t${lit(id, stats)}: ${lit(enc, stats)},`)
  }
  lines.push('}')
  lines.push('')
  return lines.join('\n')
}

// generateFeatures 生成并列产物 capability_features_gen.go。
//
// 与 generate() 的关系：同一个源、同一套幂等纪律（文件头不写日期、按 id 排序、EOL 跟随
// 现有文件），但**表与解析契约完全独立**——旧表的「恰好三段」契约一点没动，能力位另开一张表。
function generateFeatures(table, facts, stats, tally) {
  const ids = [...table.keys()].sort()
  const lines = [
    '// Code generated by scripts/sync-ability-catalog.mjs from the capability feature sources. DO NOT EDIT.',
    '',
    'package core',
    '',
    '// 由 scripts/sync-ability-catalog.mjs 生成，按「小写精确模型 id」索引。',
    '// 格式与解析契约见 capability_features.go 的 parseFeatureEncoding；本表与',
    '// capability_catalog_gen.go 的 `ctx|out|levels` 表**彼此独立**，互不解析对方。',
    '//',
    '// 值编码为 `<input>|<output>|<flags>` 三段：',
    '//   input   逗号分隔的输入模态，规范序 text,image,video,audio,pdf；未知写空段',
    '//   output  逗号分隔的输出模态，规范序 text,audio,image,video,pdf；未知写空段',
    "//   flags   **恰好 6 个字符**，固定序 tjwmaT，每字符 '1'（已知为真）/ '-'（未知）：",
    '//           t=ToolCall  j=StructuredOutput  w=NativeWebSearch',
    '//           m=MidConvSystem  a=Attachment  T=Temperature',
    '//',
    '// 只收录「至少有一位已知为真」的 id；解析端 fail-closed：段数不为 3、flags 长度不为 6',
    '// 一律返回零值。本表只承载「已知为真」的正向事实，\'-\' 表示**未知**（不是假），',
    '// 因此消费端绝不能把 \'-\' 当成 false 写进目标 harness 的配置。',
    '//',
    '// 数据来源（两份，均为只读）：',
    `//   1. DSH pi-ai 离线目录 ${facts.piDir}`,
    `//      manifest structureHash: ${facts.manifestHash}（manifest generatedAt ${facts.manifestGeneratedAt}）`,
    `//      已按 manifest 的 SHA-256 逐文件校验 ${facts.verifiedFiles} 个 provider 文件。`,
    '//      表索引取每条记录自己的 `id` 字段（目录键带 `chat:` / `image:` / `classifier:` 类型',
    '//      前缀，配置与 API 响应里都没有），且只收 `chat` 类型的记录；',
    '//      只取 input[]（模态）与 compat 的**显式白名单**三键：supportsStrictMode（结构化输出）、',
    '//      supportsTemperature（温度）、supportsMidConvoSystemMessages（中段 system）；其余 compat 键',
    '//      一律不映射（宁可少不可错）。',
    `//   2. models.dev ${facts.snapshotSource}`,
    `//      快照拉取时刻 ${facts.snapshotFetchedAt}；快照位于 ${SNAPSHOT_REL}`,
    '//      只取 modalities{input,output}、tool_call、structured_output、attachment、temperature。',
    '//',
    '// 合并规则：只按数据源分层——pi-ai 精确表 + models.dev 取**并集**（模态数组按词并集、',
    '// 布尔位取或：模态是「已知支持」的正向集合，pi-ai 的词表只有 text/image，缺项不等于否定）；',
    '// 全部小写化后建表，两层都没有任何已知能力位的 id 不落表。',
    `// 候选条目：pi-ai ${stats.piCandidates} 条、models.dev ${stats.mdCandidates} 条；去重合并后本表共 ${ids.length} 条唯一 id：`,
    `//   仅 pi-ai ${stats.piOnly} 条、两层都有 ${stats.both} 条、仅 models.dev ${stats.mdOnly} 条。`,
    '// 位覆盖（逐位计数，各数之和大于表条数）：',
    `//   t(工具调用)=${tally.t}  j(结构化输出)=${tally.j}  w(联网搜索)=${tally.w}`,
    `//   m(中段 system)=${tally.m}  a(附件)=${tally.a}  T(温度)=${tally.T}`,
    `//   输入模态：含 text ${tally.inputText} 条、含 image ${tally.inputImage} 条；输出模态非空 ${tally.withOutput} 条。`,
    '//',
    '// 文件头故意不写抽取日期：--check 做整文件字节比较，写上「今天」会让跨天的校验无故失败；',
    '// 上面两个来源摘要都只随真实数据变化（models.dev 的抓取时刻来自快照文件本身）。',
    '',
    'var capabilityFeatureTable = map[string]string{',
  ]
  for (const id of ids) {
    const e = table.get(id)
    lines.push(`\t${lit(id, facts.featureStats)}: ${lit(encodeFeatures(e), facts.featureStats)},`)
  }
  lines.push('}')
  lines.push('')
  return lines.join('\n')
}

// ---- 主流程 ------------------------------------------------------------------

const piDir = readAsarDir(asarPath, PI_AI_SEGMENTS)
const { manifest, checked } = verifyManifest(piDir)
const pi = extractPiAI(piDir)

const snapshotAbs = join(root, SNAPSHOT_REL)
let snapshot
let snapshotFetched = false

if (updateSnapshot) {
  const res = await fetchModelsDev()
  if (res.text) {
    let api
    try {
      api = JSON.parse(res.text)
    } catch (e) {
      console.error(`models.dev 响应不是合法 JSON：${String(e && e.message)}`)
      process.exit(2)
    }
    const built = buildSnapshot(api)
    const now = new Date().toISOString()
    snapshot = {
      ...snapshotMeta(
        MODELS_DEV_URL,
        now,
        '裁剪自 models.dev api.json：仅保留无命名空间前缀（不含 "/"）的模型 id；levels 取自 ' +
          'reasoning_options[type=="effort"].values；能力位取 modalities{input,output}（只留词表内取值、按规范序）' +
          '与 tool_call/structured_output/attachment/temperature（**仅为 true 时写键**，缺键表示未知或假）。'
      ),
      providers: built.providers,
    }
    mkdirSync(dirname(snapshotAbs), { recursive: true })
    writeFileSync(snapshotAbs, JSON.stringify(snapshot, null, 0) + '\n')
    snapshotFetched = true
    console.log(`已刷新 ${SNAPSHOT_REL}（经由 ${res.via}）：${built.kept} 条模型，跳过 ${built.dropped} 条。`)
  } else {
    console.error('刷新 models.dev 快照失败：')
    for (const e of res.errors || []) console.error(`  - ${e}`)
    if (!existsSync(snapshotAbs)) {
      snapshot = { ...snapshotMeta(MODELS_DEV_URL, '', '未联网取得数据：空快照占位，仅 pi-ai 部分参与生成。'), providers: {} }
      mkdirSync(dirname(snapshotAbs), { recursive: true })
      writeFileSync(snapshotAbs, JSON.stringify(snapshot, null, 0) + '\n')
      console.error(`  已写入空快照占位：${SNAPSHOT_REL}（只有 pi-ai 参与生成）。`)
    } else {
      console.error(`  保留既有快照 ${SNAPSHOT_REL}，不改动。`)
    }
    process.exit(1)
  }
}

if (!snapshot) {
  if (!existsSync(snapshotAbs)) {
    console.error(`未找到 models.dev 快照：${SNAPSHOT_REL}`)
    console.error('请先联网运行 node scripts/sync-ability-catalog.mjs --update-snapshot。')
    process.exit(2)
  }
  snapshot = JSON.parse(readFileSync(snapshotAbs, 'utf8'))
}
const mdProviders = (snapshot && snapshot.providers) || {}

const { chosen, stats } = merge(pi.entries, mdProviders)
const { table: featureTable, stats: featureStats } = mergeFeatures(pi.entries, mdProviders)
const featureTally = featureBitTally(featureTable)

// --verify：只读交叉校验，做完即退出（不写文件、不触碰生成物）。
if (verifyOnly) {
  await verifyAgainstOpenRouter(chosen)
  process.exit(0)
}

const piUnique = new Set(pi.entries.map((e) => e.id)).size
const mdUnique = Object.values(mdProviders).reduce((n, m) => n + Object.keys(m).length, 0)

const facts = {
  piDir: `${asarPath}!/${PI_AI_SEGMENTS.join('/')}`,
  manifestHash: manifest.structureHash || '(未提供)',
  manifestGeneratedAt: manifest.generatedAt || '(未提供)',
  verifiedFiles: checked,
  snapshotSource: (snapshot && snapshot.source) || MODELS_DEV_URL,
  snapshotFetchedAt: (snapshot && snapshot.fetchedAt) || '(未提供)',
  originalVendors: [...ORIGINAL_VENDORS].join(', '),
  piUnique,
  mdUnique,
  // origin 为 'models.dev' 的是快照来源，其余是 pi-ai 的文件名。
  piWinners: [...chosen.values()].filter((e) => e.origin !== 'models.dev').length,
  // 能力位表的反引号退化计数与上面那张表分开，方便定位是哪一个产物出了问题。
  featureStats: { backtickFallback: 0 },
}

const next = generate(chosen, facts, stats)
const targetAbs = join(root, TARGET_REL)
const eol = existsSync(targetAbs) ? eolOf(readFileSync(targetAbs, 'utf8')) : '\r\n'
const nextBytes = Buffer.byteLength(next.split('\n').join(eol))
const current = existsSync(targetAbs) ? readFileSync(targetAbs, 'utf8') : ''

const featuresNext = generateFeatures(featureTable, facts, featureStats, featureTally)
const featuresTargetAbs = join(root, FEATURES_TARGET_REL)
const featuresEol = existsSync(featuresTargetAbs) ? eolOf(readFileSync(featuresTargetAbs, 'utf8')) : '\r\n'
const featuresNextBytes = Buffer.byteLength(featuresNext.split('\n').join(featuresEol))
const featuresCurrent = existsSync(featuresTargetAbs) ? readFileSync(featuresTargetAbs, 'utf8') : ''

const byProvider = {}
for (const e of chosen.values()) byProvider[e.provider] = (byProvider[e.provider] || 0) + 1
const top = Object.entries(byProvider).sort((a, b) => b[1] - a[1]).slice(0, 10)

const summary = () => {
  console.log(`目录条数：${chosen.size}`)
  console.log(`生成文件字节数：${nextBytes}`)
  console.log(`来源：pi-ai ${facts.piWinners} 条，models.dev ${chosen.size - facts.piWinners} 条`)
  console.log(`冲突丢弃：${stats.conflicts}，无信息跳过：${stats.empty}${stats.backtickFallback ? `，反引号退化：${stats.backtickFallback}` : ''}`)
  console.log(`来源分布（前 10）：${top.map(([p, n]) => `${p}:${n}`).join(' ')}`)
  console.log(`pi-ai manifest 校验：${checked} 个文件全部通过（structureHash ${facts.manifestHash}）`)
  if (pi.skippedNonChat > 0) {
    console.log(`pi-ai 非对话记录跳过：${pi.skippedNonChat} 条（image / classifier，不进本表）`)
  }
  // 能力位表：逐位计数（一个 id 可以同时点亮多位），用于人工核对覆盖率。
  console.log(`能力位表条数：${featureTable.size}（候选条目 pi-ai ${featureStats.piCandidates}、models.dev ${featureStats.mdCandidates}；`
    + `无能力位跳过 ${featureStats.skippedEmpty}；仅 pi-ai ${featureStats.piOnly}、两层都有 ${featureStats.both}、仅 models.dev ${featureStats.mdOnly}）`)
  console.log(`模态集合因并集而变大的 id：${featureStats.unionGrew}`)
  console.log(`能力位表字节数：${featuresNextBytes}`)
  console.log(`逐位为真：t(工具调用)=${featureTally.t} j(结构化输出)=${featureTally.j} w(联网搜索)=${featureTally.w} `
    + `m(中段 system)=${featureTally.m} a(附件)=${featureTally.a} T(温度)=${featureTally.T}`)
  console.log(`模态：输入含 text=${featureTally.inputText} 含 image=${featureTally.inputImage}，输出模态非空=${featureTally.withOutput}`)
}

if (checkOnly) {
  // 两个产物都校验**再**退出，否则「先不一致的那个」会掩盖另一个的状态。
  const catalogOk = current === next.split('\n').join(eol)
  const featuresOk = featuresCurrent === featuresNext.split('\n').join(featuresEol)
  if (catalogOk) {
    console.log(`能力目录已同步（${chosen.size} 条）`)
  } else {
    console.error(`${TARGET_REL} 与数据源不一致，请运行 node scripts/sync-ability-catalog.mjs：`)
    console.error(`  - 期望 ${nextBytes} 字节，磁盘 ${Buffer.byteLength(current)} 字节`)
  }
  if (featuresOk) {
    console.log(`能力位表已同步（${featureTable.size} 条）`)
  } else {
    console.error(`${FEATURES_TARGET_REL} 与数据源不一致，请运行 node scripts/sync-ability-catalog.mjs：`)
    console.error(`  - 期望 ${featuresNextBytes} 字节，磁盘 ${Buffer.byteLength(featuresCurrent)} 字节`)
  }
  if (!catalogOk || !featuresOk) process.exit(1)
  summary()
  process.exit(0)
}

if (current === next.split('\n').join(eol)) {
  console.log(`能力目录已一致，无需改动（${chosen.size} 条）`)
  summary()
} else {
  // 生成前先把条数与文件字节数打出来，便于在仓库体积失控前叫停。
  console.log(`即将写入 ${TARGET_REL}：${chosen.size} 条，${nextBytes} 字节`)
  summary()
  writeFileSync(targetAbs, next.split('\n').join(eol))
  console.log(`已生成 ${TARGET_REL}：${chosen.size} 条`)
}
if (featuresCurrent === featuresNext.split('\n').join(featuresEol)) {
  console.log(`能力位表已一致，无需改动（${featureTable.size} 条）`)
} else {
  console.log(`即将写入 ${FEATURES_TARGET_REL}：${featureTable.size} 条，${featuresNextBytes} 字节`)
  writeFileSync(featuresTargetAbs, featuresNext.split('\n').join(featuresEol))
  console.log(`已生成 ${FEATURES_TARGET_REL}：${featureTable.size} 条`)
}
if (snapshotFetched) console.log(`models.dev 快照已刷新：${SNAPSHOT_REL}`)
if (stats.conflictsByKey.length > 0) {
  console.log('冲突样例（最多 20 条）：')
  for (const s of stats.conflictsByKey) console.log(`  - ${s}`)
}
