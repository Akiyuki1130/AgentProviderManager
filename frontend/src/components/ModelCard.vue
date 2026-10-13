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
            <button class="switch" :class="{ on: card.reasoning }" role="switch" :aria-checked="card.reasoning" aria-label="思考" @click="toggleReasoning" />
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
                <button class="switch" :class="{ on: card.reasoning }" role="switch" :aria-checked="card.reasoning" aria-label="思考/推理" @click="toggleReasoning" />
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
                  :class="{ checked: (card.variants || []).includes(v) }"
                  @click="toggleVariant(v)"
                >
                  <span>{{ v }}</span>
                </button>
              </div>
            </div>
            <div v-if="card.reasoning" class="mc-effort">
              <label>默认档位</label>
              <select class="field select-sm" :value="card.default_variant" @change="onDefaultChange">
                <option value="">—</option>
                <option v-for="v in variantOptions" :key="v" :value="v">{{ v }}</option>
              </select>
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
import type { ModelCard } from '../types'
import { useSettingStore } from '../stores/setting'
import { highlightSegments } from '../composables/useHighlight'

const props = defineProps<{ card: ModelCard; showSelect?: boolean; kind?: string; agent?: string; query?: string }>()
const emit = defineEmits<{ (e: 'delete'): void; (e: 'update'): void }>()
const settingStore = useSettingStore()

const expanded = ref(false)
// B9 搜索命中分段（无查询时短路，零开销）
const nameSegs = computed(() => highlightSegments(props.card.name || props.card.model_id, props.query || ''))
const idSegs = computed(() => highlightSegments(props.card.model_id, props.query || ''))
const variantOptions = computed(() => props.agent === 'deepseek' ? ['off', 'low', 'high', 'max'] : ['off', 'low', 'medium', 'high', 'xhigh', 'max'])

function toggleExpand() { expanded.value = !expanded.value }
function onSelectChange(e: Event) { props.card.select = (e.target as HTMLInputElement).checked; emit('update') }
function onNameInput(e: Event) { props.card.name = (e.target as HTMLInputElement).value }
function toggleReasoning() {
  props.card.reasoning = !props.card.reasoning
  props.card._raw_reasoning_enabled = props.card.reasoning
  if (props.card.reasoning && (!props.card.variants || props.card.variants.length === 0)) {
    // 选项「单模型思考默认全部强度」开启时勾选全部档位，否则保持默认 off/high/max
    props.card.variants = settingStore.reasoningAllIntensities ? [...variantOptions.value] : ['off', 'high', 'max']
    props.card.default_variant = 'high'
  }
  if (!props.card.reasoning) { props.card.variants = []; props.card.default_variant = '' }
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
function onContextInput(e: Event) { props.card.context = (e.target as HTMLInputElement).value as unknown as string | number; }
function onOutputInput(e: Event) { props.card.output = (e.target as HTMLInputElement).value as unknown as string | number; }
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
.mc-quick { display: flex; align-items: flex-end; gap: 10px; flex-shrink: 0; }
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
/* B10/C3/D2：原 scoped 的 .effort-opt/.field/.switch/.mc-check/.check-mark 与全局控件规范层重复，已删除改用全局类。
   全局 .switch 轨道为 color-mix(text 22%)，dark 下不再隐形（原 rgba(0,0,0,.18) 硬伤随之消失）；
   全局 .check-mark 边框跟主题（原 #888 死色删除）；.effort-opt 与全局 .pill 视觉完全一致，故本地定义全删。
   D9 :active 微反馈已提升入全局规范层（.switch/.pill transform 并列），此处不再本地补齐。 */
/* 本地尺寸变体：快捷行 / 下拉用紧凑规格（全局 .field 为 32px 标准高） */
.field-sm { height: 26px; font-size: 12px; padding: 0 8px; }
.select-sm { min-width: 100px; height: 26px; font-size: 12px; }
@media (max-width: 900px) { .mc-quick { display: none; } }
@media (max-width: 700px) { .mc-params { grid-template-columns: repeat(2, 1fr); } .mc-field.span-2 { grid-column: span 2; } }
</style>
