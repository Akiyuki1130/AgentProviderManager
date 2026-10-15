<template>
  <div class="model-card" :class="{ expanded }">
    <div class="mc-top">
      <label v-if="showSelect" class="check"><input type="checkbox" :checked="card.select" aria-label="选择该模型" @change="onSelectChange" /> <span class="check-mark" /></label>
      <div class="mc-title" :title="expanded ? '收起' : '展开'" @click="toggleExpand">
        <span class="mc-name"><template v-for="(seg, si) in nameSegs" :key="si"><mark v-if="seg.hit">{{ seg.text }}</mark><span v-else>{{ seg.text }}</span></template></span>
        <span class="mc-id mono"><template v-for="(seg, si) in idSegs" :key="si"><mark v-if="seg.hit">{{ seg.text }}</mark><span v-else>{{ seg.text }}</span></template></span>
      </div>
      <Transition name="mc-quick">
        <div v-if="!expanded" class="mc-quick">
          <div class="mc-field mc-field-sm">
            <label>上下文长度</label>
            <input class="field field-sm tnum" :value="card.context" @input="onContextInput" placeholder="留空自动匹配" />
          </div>
          <div class="mc-field mc-field-sm">
            <label>输出长度</label>
            <input class="field field-sm tnum" :value="card.output" @input="onOutputInput" placeholder="留空自动匹配" />
          </div>
          <div class="mc-field mc-field-sm">
            <label>思考</label>
            <button
              class="switch"
              :class="{ on: card.reasoning }"
              role="switch"
              :aria-checked="card.reasoning"
              :aria-disabled="!canToggleReasoning"
              :disabled="!canToggleReasoning"
              :title="canToggleReasoning ? undefined : t('reasoning.noOffTitle', lang)"
              aria-label="思考"
              @click="toggleReasoning"
            />
          </div>
        </div>
      </Transition>
      <button class="mc-chevron" :class="{ open: expanded }" :title="expanded ? '收起' : '展开'" @click="toggleExpand">
        <n-icon :component="ChevronDown" :size="16" />
      </button>
      <button class="mc-del" title="删除" @click="emit('delete')">
        <n-icon :component="CloseOutline" :size="16" />
      </button>
    </div>
    <div class="fi-collapse">
      <Transition name="fi-collapse">
        <div v-show="expanded" class="mc-body">
          <div class="mc-hint">{{ card.api_returned ? 'API已返回 · ' : '' }}参数自动推断，可手动修改</div>
          <div class="mc-params">
            <div class="mc-field span-2">
              <label>显示名称</label>
              <input class="field" :value="card.name" @input="onNameInput" />
            </div>
            <div class="mc-field">
              <label>思考/推理</label>
              <div class="switch-row">
                <button
                  class="switch"
                  :class="{ on: card.reasoning }"
                  role="switch"
                  :aria-checked="card.reasoning"
                  :aria-disabled="!canToggleReasoning"
                  :disabled="!canToggleReasoning"
                  :title="canToggleReasoning ? undefined : t('reasoning.noOffTitle', lang)"
                  aria-label="思考/推理"
                  @click="toggleReasoning"
                />
              </div>
            </div>
            <div v-if="card.reasoning" class="mc-effort">
              <label>推理档位</label>
              <div class="mc-effort-opts">
                <button
                  v-for="v in variantOptions"
                  :key="v"
                  type="button"
                  class="pill"
                  :class="{ checked: (card.variants || []).includes(v), unsupported: !isSupported(v) }"
                  :title="isSupported(v) ? undefined : t('reasoning.supportedHint', lang)"
                  @click="toggleVariant(v)"
                >
                  <span>{{ v }}</span>
                </button>
              </div>
              <!-- C1：仅在确有档位被弱化（支持集与显示全集不同）时显示，避免常态噪音 -->
              <span v-if="showSupportHint" class="mc-effort-note">{{ t('reasoning.supportedHint', lang) }}</span>
            </div>
            <!-- C2：允许自填档位时露出自由输入；失焦/回车把解析出的档位名并入集合（此处不做白名单过滤，后端依开关放宽校验） -->
            <div v-if="card.reasoning && settingStore.customVariants" class="mc-effort">
              <label>{{ t('reasoning.customLabel', lang) }}</label>
              <input
                v-model="customInput"
                class="field mc-custom-input"
                :maxlength="MAX_VARIANT_LEN"
                :placeholder="t('reasoning.customPlaceholder', lang)"
                @keydown.enter.prevent="commitCustomVariants"
                @blur="commitCustomVariants"
              />
            </div>
            <div v-if="card.reasoning" class="mc-effort">
              <label>默认档位</label>
              <select class="field select-sm" :value="card.default_variant" @change="onDefaultChange">
                <option value="">—</option>
                <option v-for="v in variantOptions" :key="v" :value="v">{{ v }}</option>
              </select>
              <!-- ZCode 的 modelRule 没有「当前档位」字段：档位由 optionSpecs.reasoningLevel.map
                   在运行时求值，配置里只存可选集合与映射，默认档位无处落盘。DSH 有对应字段故有效。 -->
              <span v-if="showZcodeNote" class="mc-effort-note">{{ t('reasoning.zcodeDefaultNote', lang) }}</span>
            </div>
            <div class="mc-field">
              <label>上下文长度 (tokens)</label>
              <input class="field tnum" :value="card.context" @input="onContextInput" placeholder="留空自动匹配" />
            </div>
            <div class="mc-field">
              <label>输出长度 (tokens)</label>
              <input class="field tnum" :value="card.output" @input="onOutputInput" placeholder="留空自动匹配" />
            </div>
            <div class="mc-tip">提示：API 通常只返回模型 ID，以上参数已根据模型名自动推断。上下文与输出长度各自独立，填写即写入、留空不写入。</div>
            <!-- 阶段 E3「能力」区：harness 中立能力的唯一编辑入口，只读写 card.capabilities。
                 三态语义（键不存在 = 未知）决定了交互：勾选写 true，取消勾选删键，绝不写 false。
                 opencode 形态的旧字段 attachment / modalities 不在此区展示，也不做迁移。 -->
            <div class="mc-caps">
              <div class="mc-caps-head">
                <span class="mc-caps-title">{{ t('cap.title', lang) }}</span>
                <!-- 三态口径整块只说一次：未选择 / 未勾选 = 未知，交给目标客户端默认值。
                     不承诺「取消勾选 = 告诉客户端不支持」——写端从不下发 false，那种承诺做不到。 -->
                <span class="mc-caps-note">{{ t('cap.emptyHint', lang) }}</span>
                <span v-if="inferredSource" class="mc-caps-note">{{ t('cap.sourceLabel', lang).replace('{source}', inferredSource) }}</span>
              </div>
              <div class="mc-caps-row">
                <span class="mc-caps-label">{{ t('cap.input', lang) }}</span>
                <div class="mc-effort-opts">
                  <button
                    v-for="m in CAP_INPUT_MODALITIES"
                    :key="'in-' + m"
                    type="button"
                    class="pill"
                    :class="{ checked: hasModality('input', m), inferred: isInferredModality('input', m) }"
                    :aria-pressed="hasModality('input', m)"
                    :title="isInferredModality('input', m) ? t('cap.inferred', lang) : undefined"
                    @click="toggleModality('input', m)"
                  >
                    <span>{{ modalityLabel(m) }}</span>
                  </button>
                </div>
                <button v-if="adoptableCount('input') > 0" type="button" class="pill mc-caps-adopt" @click="adoptModalities('input')">{{ t('cap.adopt', lang) }}</button>
              </div>
              <div class="mc-caps-row">
                <span class="mc-caps-label">{{ t('cap.output', lang) }}</span>
                <div class="mc-effort-opts">
                  <button
                    v-for="m in CAP_OUTPUT_MODALITIES"
                    :key="'out-' + m"
                    type="button"
                    class="pill"
                    :class="{ checked: hasModality('output', m), inferred: isInferredModality('output', m) }"
                    :aria-pressed="hasModality('output', m)"
                    :title="isInferredModality('output', m) ? t('cap.inferred', lang) : undefined"
                    @click="toggleModality('output', m)"
                  >
                    <span>{{ modalityLabel(m) }}</span>
                  </button>
                </div>
                <button v-if="adoptableCount('output') > 0" type="button" class="pill mc-caps-adopt" @click="adoptModalities('output')">{{ t('cap.adopt', lang) }}</button>
              </div>
              <div class="mc-caps-block">
                <span class="mc-caps-label">{{ t('cap.flags', lang) }}</span>
                <div class="mc-caps-flags">
                  <!-- 自绘 .check（与卡片选择框同一控件体系）；勾选 = 写 true，取消 = 删键 -->
                  <div v-for="f in CAP_FLAG_KEYS" :key="f" class="mc-caps-flag">
                    <label class="check mc-caps-flag-toggle">
                      <input type="checkbox" :checked="flagValue(f)" :aria-label="flagLabel(f)" @change="onFlagChange(f, $event)" />
                      <span class="check-mark" />
                      <span class="mc-caps-flag-name">{{ flagLabel(f) }}</span>
                    </label>
                    <!-- 推断有值而用户未确认时标出「推断支持」，采纳按钮显式写入，不自动改数据 -->
                    <template v-if="isInferredFlag(f)">
                      <span class="mc-caps-note">{{ t('cap.inferred', lang) }}</span>
                      <button
                        type="button"
                        class="pill mc-caps-adopt"
                        :title="t('cap.adoptAria', lang).replace('{name}', flagLabel(f))"
                        :aria-label="t('cap.adoptAria', lang).replace('{name}', flagLabel(f))"
                        @click="adoptFlag(f)"
                      >{{ t('cap.adopt', lang) }}</button>
                    </template>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { NIcon } from 'naive-ui'
import { ChevronDown, CloseOutline } from '@vicons/ionicons5'
import type { ModelCapabilities, ModelCard } from '../types'
import { CAP_FLAG_KEYS, CAP_INPUT_MODALITIES, CAP_OUTPUT_MODALITIES } from '../types'
import { useSettingStore } from '../stores/setting'
import { highlightSegments } from '../composables/useHighlight'
import { t } from '../i18n'
import { REASONING_VARIANTS, REASONING_VARIANTS_BASIC, MAX_VARIANT_LEN, pickDefaultVariant } from '../constants/variants'

const props = defineProps<{ card: ModelCard; showSelect?: boolean; kind?: string; agent?: string; query?: string }>()
const emit = defineEmits<{ (e: 'delete'): void; (e: 'update'): void }>()
const settingStore = useSettingStore()
const lang = computed(() => settingStore.resolvedLang)

const expanded = ref(false)
// C2：自填档位输入框内容（逗号或换行分隔），提交时解析并入 card.variants
const customInput = ref('')
// B9 搜索命中分段（无查询时短路，零开销）
const nameSegs = computed(() => highlightSegments(props.card.name || props.card.model_id, props.query || ''))
const idSegs = computed(() => highlightSegments(props.card.model_id, props.query || ''))
// M3：展示子集 REASONING_VARIANTS（7 档）+ 当集合本身含 enabled 时并入 enabled，
// 让 ["disabled","enabled"] / ["enabled"] 这类二值模型的 enabled 档可见、可编辑。
// 来自单一真源 constants/variants.ts（DSH 与 ZCode 统一到同一展示集，不按 agent 分支）。
const baseOptions = computed<string[]>(() => {
  const opts: string[] = [...REASONING_VARIANTS]
  if ((props.card.variants || []).includes('enabled') && !opts.includes('enabled')) opts.push('enabled')
  return opts
})
// C2：默认档位下拉/胶囊还要并入用户自填的、不在内建全集里的档位，让它们可选
const variantOptions = computed<string[]>(() => {
  const opts = [...baseOptions.value]
  for (const v of props.card.variants || []) if (!opts.includes(v)) opts.push(v)
  return opts
})
// C1：某档位是否被支持＝_supported_levels 存在（非空）且包含该档位。未推断到（undefined/空）不弱化。
// 用户自填的档位不在内建推断域内，一律按支持处理（不因推断缺失而误标为弱化）。
function isSupported(v: string): boolean {
  const lv = props.card._supported_levels
  if (!lv || lv.length === 0) return true
  if (!baseOptions.value.includes(v)) return true
  return lv.includes(v)
}
// C1：仅在确有内建档位被弱化（支持集与显示全集不同）时显示说明，避免常态噪音
const showSupportHint = computed(() => {
  const lv = props.card._supported_levels
  if (!lv || lv.length === 0) return false
  return baseOptions.value.some((v) => !lv.includes(v))
})
// S1 配套：集合非空且不含 off 词（如 glm-5.3 = [low,high,max]、kimi-k2.7-code = [enabled]）
// 的模型无法表达「关闭思考」，禁用开关。集合为空属未初始化的新模型，仍可正常切换。
const canToggleReasoning = computed(() => {
  const v = props.card.variants
  return !v || v.length === 0 || v.includes('off')
})
// M7：agent 由 App.vue 异步回读；回读结束前（agentResolved=false）不渲染该提示，
// 避免首帧默认值与实际 agent 不一致导致的闪现。仅在确定是 ZCode 时显示。
const showZcodeNote = computed(() => settingStore.agentResolved && props.agent === 'zcode')

function toggleExpand() { expanded.value = !expanded.value }
function onSelectChange(e: Event) { props.card.select = (e.target as HTMLInputElement).checked; emit('update') }
function onNameInput(e: Event) { props.card.name = (e.target as HTMLInputElement).value }
function toggleReasoning() {
  // 无 off 词的模型无法关闭思考；开关已 disabled，这里再兜一层防御。
  if (!canToggleReasoning.value) return
  props.card.reasoning = !props.card.reasoning
  props.card._raw_reasoning_enabled = props.card.reasoning
  if (!props.card.reasoning) {
    // S1：关闭思考 = 档位集合收敛为单元素 off 词 ["off"]（ZCode/DSH 都靠选中 off 档表达关闭）。
    // 保留 _off_alias（写回时按原拼法落盘）与 default_variant 不动。
    props.card.variants = ['off']
  } else {
    // S1：开启思考时，若集合不含非 off 档位（为空或仅 ["off"]），按「单模型思考默认全部强度」
    // 写入全部档位或基础三档兜底，并挑一个合理默认档位。
    const cur = props.card.variants || []
    if (!cur.some((v) => v !== 'off')) {
      // C1：优先用后端推断的支持档位（非空）作为种子；为空才回退到「全集/基础三档」的既有行为。
      const supported = props.card._supported_levels
      const next = supported && supported.length > 0
        ? [...supported]
        : (settingStore.reasoningAllIntensities ? [...REASONING_VARIANTS] : [...REASONING_VARIANTS_BASIC])
      props.card.variants = next
      props.card.default_variant = pickDefaultVariant(next)
    }
  }
  emit('update')
}
function toggleVariant(v: string) {
  const arr = props.card.variants || []
  const idx = arr.indexOf(v)
  if (idx >= 0) arr.splice(idx, 1)
  else arr.push(v)
  props.card.variants = [...arr]
  if (props.card.default_variant && !arr.includes(props.card.default_variant)) {
    props.card.default_variant = arr[0] || ''
  }
  emit('update')
}
function onDefaultChange(e: Event) { props.card.default_variant = (e.target as HTMLSelectElement).value; emit('update') }
// C2：自填输入解析成档位名并入 card.variants。逗号或换行分隔，trim 去空、去重；
// 与后端 canonicalVariant 一致地做别名归一（none/disabled/nothink→off，大小写归一），
// 避免同一个 off 档以 off/disabled/Disabled 多个等价拼法并存。此处不做白名单过滤——
// 后端依 customVariants 开关决定是否放宽；超长档位名由后端 NormalizeVariants 兜底拒绝。
function normVariant(s: string): string {
  const v = s.trim().toLowerCase()
  return v === 'none' || v === 'disabled' || v === 'nothink' ? 'off' : v
}
function commitCustomVariants() {
  const raw = customInput.value
  if (!raw.trim()) { customInput.value = ''; return }
  const parsed = raw
    .split(/[,\n\r]+/)
    .map((s) => normVariant(s))
    .filter((s) => s.length > 0 && s.length <= MAX_VARIANT_LEN)
  if (parsed.length === 0) { customInput.value = ''; return }
  const arr = [...(props.card.variants || [])]
  for (const v of parsed) if (!arr.includes(v)) arr.push(v)
  props.card.variants = arr
  customInput.value = ''
  emit('update')
}
function onContextInput(e: Event) { props.card.context = (e.target as HTMLInputElement).value as unknown as string | number; }
function onOutputInput(e: Event) { props.card.output = (e.target as HTMLInputElement).value as unknown as string | number; }

// ---- 阶段 E3：能力区 ----
type ModalityDir = 'input' | 'output'
type FlagKey = (typeof CAP_FLAG_KEYS)[number]

/** 用户可编辑的权威值；缺失即「未知」，不要在这里补默认值。 */
function caps(): ModelCapabilities { return props.card.capabilities || {} }
/** 只读推断值，用于标出「推断支持」，绝不回写。 */
function inferred(): ModelCapabilities { return props.card._capabilities_inferred || {} }

function modalityKey(dir: ModalityDir): 'input_modalities' | 'output_modalities' {
  return dir === 'input' ? 'input_modalities' : 'output_modalities'
}
function modalityOrder(dir: ModalityDir): readonly string[] {
  return dir === 'input' ? CAP_INPUT_MODALITIES : CAP_OUTPUT_MODALITIES
}

/**
 * 回写能力对象：整份为空对象时把 capabilities 键删掉——「什么都没填」仍是未知，
 * 留一个 `{}` 会让卡片看起来「已确认无任何能力」，与三态语义相悖。
 * 只改数据不回写后端：保存路径由页面既有的卡片载荷负责。
 */
function writeCaps(next: ModelCapabilities) {
  props.card.capabilities = Object.keys(next).length > 0 ? next : undefined
  emit('update')
}

function hasModality(dir: ModalityDir, m: string): boolean {
  return (caps()[modalityKey(dir)] || []).includes(m)
}

/** 能力表推断为支持、但用户尚未确认的空位（用于标出「推断支持」）。 */
function isInferredModality(dir: ModalityDir, m: string): boolean {
  const inf = inferred()[modalityKey(dir)]
  return !!inf && inf.includes(m) && !hasModality(dir, m)
}

/** 该方向可采纳（推断有值且当前缺位、且在规范词表内）的模态数量，0 时不渲染采纳按钮。 */
function adoptableCount(dir: ModalityDir): number {
  const inf = inferred()[modalityKey(dir)] || []
  const order = modalityOrder(dir)
  return inf.filter((m) => order.includes(m) && !hasModality(dir, m)).length
}

function toggleModality(dir: ModalityDir, m: string) {
  const key = modalityKey(dir)
  const next: ModelCapabilities = { ...caps() }
  const picked = new Set(next[key] || [])
  if (picked.has(m)) picked.delete(m)
  else picked.add(m)
  // 按规范序重建集合（去重 + 稳定顺序），空集合删键：模态是集合语义，留空数组无法表达「未知」
  const list = modalityOrder(dir).filter((x) => picked.has(x))
  if (list.length > 0) next[key] = list
  else delete next[key]
  writeCaps(next)
}

function adoptModalities(dir: ModalityDir) {
  const key = modalityKey(dir)
  const order = modalityOrder(dir)
  const next: ModelCapabilities = { ...caps() }
  const picked = new Set(next[key] || [])
  for (const m of inferred()[key] || []) if (order.includes(m)) picked.add(m)
  next[key] = order.filter((x) => picked.has(x))
  writeCaps(next)
}

function flagValue(f: FlagKey): boolean { return caps()[f] === true }
function isInferredFlag(f: FlagKey): boolean { return inferred()[f] === true && caps()[f] === undefined }

function onFlagChange(f: FlagKey, e: Event) {
  const next: ModelCapabilities = { ...caps() }
  if ((e.target as HTMLInputElement).checked) next[f] = true
  // 取消勾选 = 删键（回到未知），绝不写 false
  else delete next[f]
  writeCaps(next)
}

function adoptFlag(f: FlagKey) {
  const next: ModelCapabilities = { ...caps() }
  next[f] = true
  writeCaps(next)
}

function modalityLabel(m: string): string { return t(`cap.modal.${m}`, lang.value) }
function flagLabel(f: FlagKey): string { return t(`cap.flag.${f}`, lang.value) }

/** 推断来源只读展示（`_source` 由后端填，可能缺失）。 */
const inferredSource = computed(() => {
  const s = inferred()._source
  return typeof s === 'string' && s.trim() !== '' ? s.trim() : ''
})
</script>

<style scoped>
/* transition 必须并列列出 opacity/transform：scoped 的 .model-card 优先级高于全局 .list-*（transition: all），
   若只写 padding 会覆盖掉卡片增删/FLIP 移动动画；时长与全局 list 档对齐 */
.model-card { background: var(--fluent-card-bg); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; margin-bottom: 8px; transition: padding var(--dur-settle, 0.28s) var(--ease-standard), opacity var(--dur-standard-lg, 0.25s) ease, transform var(--dur-standard-lg, 0.25s) ease; }
.model-card.expanded { padding: 12px; }
.mc-top { display: flex; align-items: center; gap: 10px; }
.mc-title { display: flex; flex-direction: column; min-width: 0; flex: 1; cursor: pointer; }
.mc-name { font-size: 13px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.mc-id { font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
/* 列高度不一（输入 26px vs 开关 20px）：flex-end 会让开关比输入框低 3px，center 使两列控件中线严格对齐 */
.mc-quick { display: flex; align-items: center; gap: 10px; flex-shrink: 0; }
/* 收起态快捷行仅透明度渐隐（无 transform，不给滚动容器引入溢出）；时长取 leave 档，与卡片高度动画错开 */
.mc-quick-enter-active, .mc-quick-leave-active { transition: opacity var(--dur-page-leave, 0.167s) ease; }
.mc-quick-enter-from, .mc-quick-leave-to { opacity: 0; }
.mc-field-sm { width: 96px; }
.mc-chevron { width: 26px; height: 26px; border-radius: 6px; border: 1px solid var(--fluent-border); background: transparent; color: var(--fluent-text-soft); cursor: pointer; display: inline-flex; align-items: center; justify-content: center; transition: transform 0.2s, border-color 0.2s, color 0.2s; flex-shrink: 0; }
.mc-chevron:hover { border-color: var(--accent); color: var(--accent); }
.mc-chevron.open { transform: rotate(180deg); }
.mc-del { color: var(--err); width: 26px; height: 26px; border-radius: 6px; border: 1px solid var(--fluent-border); background: transparent; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; }
.mc-del:hover { background: var(--err-soft); }
.mc-hint { font-size: 11px; color: var(--fluent-text-soft); margin: 6px 0 8px; }
/* 参数展开高度动画（interpolate-size 主路径；低版本 grid 回退见下方 @supports，与参考实现同款三件套） */
.mc-body { overflow: hidden; } /* mc-hint 的上边距被包进主体，收起动画结束时无残留高度 */
.fi-collapse { overflow: hidden; }
.fi-collapse-enter-active, .fi-collapse-leave-active { overflow: hidden; transition: height var(--dur-settle, 0.28s) var(--ease-standard), opacity var(--dur-settle, 0.28s) var(--ease-standard); }
.fi-collapse-enter-from, .fi-collapse-leave-to { height: 0; opacity: 0; }
@supports not (interpolate-size: allow-keywords) {
  .fi-collapse { display: grid; grid-template-rows: 0fr; align-items: start; transition: grid-template-rows var(--dur-settle, 0.28s) var(--ease-standard); }
  .fi-collapse:has(.fi-collapse-enter-active, .fi-collapse-leave-to) { grid-template-rows: 1fr; }
  .fi-collapse > * { min-height: 0; overflow: hidden; }
}
.mc-params { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 8px; }
.mc-params > *, .mc-field { min-width: 0; }
.mc-field { display: flex; flex-direction: column; gap: 2px; }
.mc-field.span-2 { grid-column: span 2; }
.mc-field label { min-width: 0; font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.mc-field .field { height: 30px; font-size: 12px; }
.mc-field .switch-row { display: flex; align-items: center; height: 30px; }
.mc-tip { grid-column: 1 / -1; font-size: 10px; color: var(--fluent-text-soft); }
.mc-effort { grid-column: 1 / -1; display: flex; align-items: center; gap: 8px; background: color-mix(in srgb, var(--accent, #0078d4) 8%, transparent); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; }
.mc-effort label { font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; }
.mc-effort-opts { display: flex; flex-wrap: wrap; gap: 6px; flex: 1; }
/* C2：自填档位输入框在 .mc-effort 的 flex 行内占满剩余宽度（全局 .field 为 100% 宽，需在 flex 下归一） */
.mc-custom-input { flex: 1; min-width: 0; height: 30px; font-size: 12px; }
.mc-effort-note { font-size: 10px; color: var(--fluent-text-soft); white-space: nowrap; }
/* 阶段 E3 能力区：横跨整行参数网格的独立区块（与 .mc-effort 同为 grid-column: 1 / -1 的整行块，
   但用中性底色——能力是用户可编辑的权威值，不是「推断结果」这类提示）。 */
.mc-caps { grid-column: 1 / -1; display: flex; flex-direction: column; gap: 8px; border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; }
.mc-caps-head { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; }
.mc-caps-title { font-size: 11px; font-weight: 600; }
.mc-caps-note { font-size: 10px; color: var(--fluent-text-soft); }
.mc-caps-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.mc-caps-label { font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; }
.mc-caps-block { display: flex; flex-direction: column; gap: 6px; }
.mc-caps-flags { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px 12px; }
.mc-caps-flag { display: flex; align-items: center; gap: 6px; min-width: 0; }
.mc-caps-flag-toggle { gap: 6px; min-width: 0; }
.mc-caps-flag-name { font-size: 11px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
/* 采纳按钮复用 .pill（同屏只有一套胶囊控件体系），这里只压一档尺寸 */
.mc-caps-adopt { font-size: 10px; padding: 2px 8px; }
/* B10/C3/D2：原 scoped 的 .effort-opt/.field/.switch/.mc-check/.check-mark 与全局控件规范层重复，已删除改用全局类。
   全局 .switch 轨道为 color-mix(text 22%)，dark 下不再隐形（原 rgba(0,0,0,.18) 硬伤随之消失）；
   全局 .check-mark 边框跟主题（原 #888 死色删除）；.effort-opt 与全局 .pill 视觉完全一致，故本地定义全删。
   D9 :active 微反馈已提升入全局规范层（.switch/.pill transform 并列），此处不再本地补齐。 */
/* 本地尺寸变体：快捷行 / 下拉用紧凑规格（全局 .field 为 32px 标准高）；圆角按小控件档位 8px（≈28 档） */
.field-sm { height: 26px; font-size: 12px; padding: 0 8px; border-radius: 8px; }
.select-sm { min-width: 100px; height: 26px; font-size: 12px; border-radius: 8px; }
@media (max-width: 900px) { .mc-quick { display: none; } }
@media (max-width: 700px) { .mc-params { grid-template-columns: repeat(2, 1fr); } .mc-field.span-2 { grid-column: span 2; } .mc-caps-flags { grid-template-columns: 1fr; } }
</style>
