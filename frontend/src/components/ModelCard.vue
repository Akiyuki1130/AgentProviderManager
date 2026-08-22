<template>
  <div class="model-card" :class="{ expanded }">
    <div class="mc-top">
      <label v-if="showSelect" class="mc-check"><input type="checkbox" :checked="card.select" @change="onSelectChange" /> <span class="check-mark" /></label>
      <div class="mc-title" :title="expanded ? '收起' : '展开'" @click="toggleExpand">
        <span class="mc-name">{{ card.name || card.model_id }}</span>
        <span class="mc-id">{{ card.model_id }}</span>
      </div>
      <div v-if="!expanded" class="mc-quick">
        <div class="mc-field mc-field-sm">
          <label>上下文长度</label>
          <input class="field field-sm" :value="card.context" @input="onContextInput" placeholder="留空自动匹配" />
        </div>
        <div class="mc-field mc-field-sm">
          <label>输出长度</label>
          <input class="field field-sm" :value="card.output" @input="onOutputInput" placeholder="留空自动匹配" />
        </div>
        <div class="mc-field mc-field-sm">
          <label>思考</label>
          <button class="switch" :class="{ on: card.reasoning }" @click="toggleReasoning" />
        </div>
      </div>
      <button class="mc-chevron" :class="{ open: expanded }" :title="expanded ? '收起' : '展开'" @click="toggleExpand">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6" /></svg>
      </button>
      <button class="mc-del" title="删除" @click="emit('delete')">✕</button>
    </div>
    <template v-if="expanded">
      <div class="mc-hint">{{ card.api_returned ? 'API已返回 · ' : '' }}参数自动推断，可手动修改</div>
      <div class="mc-params">
        <div class="mc-field span-2">
          <label>显示名称</label>
          <input class="field" :value="card.name" @input="onNameInput" />
        </div>
        <div class="mc-field">
          <label>思考/推理</label>
          <div class="switch-row">
            <button class="switch" :class="{ on: card.reasoning }" @click="toggleReasoning" />
          </div>
        </div>
        <div v-if="card.reasoning" class="mc-effort">
          <label>推理档位</label>
          <div class="mc-effort-opts">
            <button
              v-for="v in variantOptions"
              :key="v"
              type="button"
              class="effort-opt"
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
        <div v-if="isOpenCode" class="mc-capabilities">
          <label><input type="checkbox" v-model="card.attachment" /> 支持附件</label>
        </div>
        <div v-if="isOpenCode" class="mc-field mc-json-field">
          <label title="输入/输出模态，使用逗号分隔">输入/输出模态（逗号分隔）</label>
          <input class="field" :value="modalitiesValue" @change="onModalitiesChange" placeholder="text,image" />
        </div>
        <div v-if="isOpenCode" class="mc-field mc-json-field">
          <label>模型 Headers JSON</label>
          <input class="field" :value="jsonValue(card.headers)" @change="onHeadersChange" placeholder='{"x-api-key":"..."}' />
        </div>
        <div v-if="isOpenCode" class="mc-field mc-json-field">
          <label>模型 Options JSON</label>
          <input class="field" :value="jsonValue(card.options)" @change="onOptionsChange" placeholder='{"reasoningEffort":"high"}' />
        </div>
        <div class="mc-field">
          <label>上下文长度 (tokens)</label>
          <input class="field" :value="card.context" @input="onContextInput" placeholder="留空自动匹配" />
        </div>
        <div class="mc-field">
          <label>输出长度 (tokens)</label>
          <input class="field" :value="card.output" @input="onOutputInput" placeholder="留空自动匹配" />
        </div>
        <div class="mc-tip">提示：API 通常只返回模型 ID，以上参数已根据模型名自动推断。上下文与输出长度各自独立，填写即写入、留空不写入。</div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { ModelCard } from '../types'
import { useSettingStore } from '../stores/setting'

const props = defineProps<{ card: ModelCard; showSelect?: boolean; kind?: string; agent?: string }>()
const emit = defineEmits<{ (e: 'delete'): void; (e: 'update'): void }>()
const settingStore = useSettingStore()

const expanded = ref(false)
const isOpenCode = computed(() => props.agent === 'opencode')
const variantOptions = computed(() => props.agent === 'deepseek' ? ['off', 'low', 'high', 'max'] : ['off', 'low', 'medium', 'high', 'xhigh', 'max'])
const supportedModalities = ['text', 'audio', 'image', 'video', 'pdf']
const modalitiesValue = computed(() => {
  const modalities = props.card.modalities
  if (!modalities || typeof modalities !== 'object') return ''
  const values = [...toModalityList(modalities.input), ...toModalityList(modalities.output)]
  return [...new Set(values)].join(',')
})
function toModalityList(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string' && supportedModalities.includes(item))
}
function jsonValue(value: unknown) { return value == null ? '' : JSON.stringify(value) }
function parseJsonField(e: Event, key: 'modalities' | 'options') { const value = (e.target as HTMLInputElement).value.trim(); if (!value) { props.card[key] = undefined; return }; try { const parsed = JSON.parse(value); if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) props.card[key] = parsed as never } catch { /* keep last valid value */ } }
function onModalitiesChange(e: Event) {
  const values = [...new Set((e.target as HTMLInputElement).value.split(',').map((item) => item.trim().toLowerCase()).filter((item) => supportedModalities.includes(item)))]
  props.card.modalities = values.length ? { input: [...values], output: [...values] } : undefined
  emit('update')
}
function onOptionsChange(e: Event) { parseJsonField(e, 'options'); emit('update') }
function onHeadersChange(e: Event) { const value = (e.target as HTMLInputElement).value.trim(); if (!value) { props.card.headers = undefined; emit('update'); return }; try { const parsed = JSON.parse(value); if (parsed && typeof parsed === 'object' && !Array.isArray(parsed) && Object.values(parsed).every((v) => typeof v === 'string')) props.card.headers = parsed as Record<string, string>; emit('update') } catch { /* keep last valid value */ } }

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
.model-card { background: var(--fluent-card-bg); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; margin-bottom: 8px; }
.model-card.expanded { padding: 12px; }
.mc-top { display: flex; align-items: center; gap: 10px; }
.mc-title { display: flex; flex-direction: column; min-width: 0; flex: 1; cursor: pointer; }
.mc-name { font-size: 13px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.mc-id { font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.mc-quick { display: flex; align-items: flex-end; gap: 10px; flex-shrink: 0; }
.mc-field-sm { width: 96px; }
.mc-chevron { width: 26px; height: 26px; border-radius: 6px; border: 1px solid var(--fluent-border); background: transparent; color: var(--fluent-text-soft); cursor: pointer; display: inline-flex; align-items: center; justify-content: center; transition: transform 0.2s, border-color 0.2s, color 0.2s; flex-shrink: 0; }
.mc-chevron:hover { border-color: var(--accent); color: var(--accent); }
.mc-chevron.open { transform: rotate(180deg); }
.mc-del { color: #d13438; width: 26px; height: 26px; border-radius: 6px; border: 1px solid var(--fluent-border); background: transparent; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; }
.mc-del:hover { background: rgba(209,52,56,0.08); }
.mc-hint { font-size: 11px; color: var(--fluent-text-soft); margin: 6px 0 8px; }
.mc-params { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 8px; }
.mc-params > *, .mc-field { min-width: 0; }
.mc-field { display: flex; flex-direction: column; gap: 2px; }
.mc-field.span-2 { grid-column: span 2; }
.mc-json-field { grid-column: 1 / -1; }
.mc-field label { min-width: 0; font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.mc-field .field { height: 30px; font-size: 12px; }
.mc-field .switch-row { display: flex; align-items: center; height: 30px; }
.mc-capabilities { grid-column: 1 / -1; display: flex; gap: 14px; align-items: center; flex-wrap: wrap; font-size: 11px; color: var(--fluent-text-soft); }
.mc-tip { grid-column: 1 / -1; font-size: 10px; color: var(--fluent-text-soft); }
.mc-effort { grid-column: 1 / -1; display: flex; align-items: center; gap: 8px; background: color-mix(in srgb, var(--accent, #0078d4) 8%, transparent); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; }
.mc-effort label { font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; }
.mc-effort-opts { display: flex; flex-wrap: wrap; gap: 6px; flex: 1; }
.effort-opt { display: inline-flex; align-items: center; justify-content: center; gap: 4px; font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 3px 10px; background: var(--fluent-card-bg); cursor: pointer; color: inherit; font-family: inherit; line-height: 1; appearance: none; -webkit-appearance: none; outline: none; }
.effort-opt:hover { border-color: var(--accent); }
.effort-opt:focus-visible { outline: none; box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 25%, transparent); }
.effort-opt.checked { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 12%, transparent); color: var(--accent); }
.effort-opt.checked:hover { border-color: var(--accent); }
.field { width: 100%; min-width: 0; box-sizing: border-box; height: 32px; border-radius: 6px; border: 1px solid var(--fluent-border); background: var(--fluent-card-bg); padding: 0 10px; font-size: 13px; color: var(--fluent-text); }
.field:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 20%, transparent); }
.field-sm { height: 26px; font-size: 12px; padding: 0 8px; }
.select-sm { min-width: 100px; height: 26px; font-size: 12px; }
.switch { position: relative; width: 40px; height: 20px; border-radius: 10px; background: rgba(0,0,0,0.18); border: none; cursor: pointer; flex-shrink: 0; }
.switch::after { content: ""; position: absolute; top: 4px; left: 4px; width: 12px; height: 12px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.3); transition: transform 0.2s; }
.switch.on { background: var(--accent); }
.switch.on::after { transform: translateX(20px); }
.mc-check { display: inline-flex; align-items: center; cursor: pointer; flex-shrink: 0; position: relative; }
.mc-check input { position: absolute; opacity: 0; }
.mc-check input:focus-visible + .check-mark { outline: 2px solid var(--accent); outline-offset: 2px; }
.check-mark { width: 18px; height: 18px; border: 1.5px solid #888; border-radius: 4px; display: inline-flex; align-items: center; justify-content: center; }
.mc-check input:checked + .check-mark { background: var(--accent); border-color: var(--accent); }
.mc-check input:checked + .check-mark::after { content: ""; width: 5px; height: 9px; border: solid #fff; border-width: 0 2px 2px 0; transform: rotate(45deg) translate(-1px, -1px); }
@media (max-width: 900px) { .mc-quick { display: none; } }
@media (max-width: 700px) { .mc-params { grid-template-columns: repeat(2, 1fr); } .mc-field.span-2 { grid-column: span 2; } }
</style>
