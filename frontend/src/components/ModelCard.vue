<template>
  <div class="model-card">
    <div class="mc-top">
      <label v-if="showSelect" class="mc-check"><input type="checkbox" :checked="card.select" @change="onSelectChange" /> <span class="check-mark" /></label>
      <span class="mc-id">{{ card.model_id }}</span>
      <button class="mc-del" title="删除" @click="emit('delete')">✕</button>
    </div>
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
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ModelCard } from '../types'

const props = defineProps<{ card: ModelCard; showSelect?: boolean; kind?: string }>()
const emit = defineEmits<{ (e: 'delete'): void; (e: 'update'): void }>()

const variantOptions = computed(() => {
  return ['off', 'low', 'medium', 'high', 'xhigh', 'max']
})

function onSelectChange(e: Event) { props.card.select = (e.target as HTMLInputElement).checked; emit('update') }
function onNameInput(e: Event) { props.card.name = (e.target as HTMLInputElement).value }
function toggleReasoning() {
  props.card.reasoning = !props.card.reasoning
  if (props.card.reasoning && (!props.card.variants || props.card.variants.length === 0)) {
    props.card.variants = ['off', 'high', 'max']
    props.card.default_variant = 'max'
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
.model-card { background: var(--fluent-card-bg); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 12px; margin-bottom: 8px; }
.mc-top { display: flex; align-items: center; gap: 8px; }
.mc-id { font-size: 13px; font-weight: 600; flex: 1; word-break: break-all; }
.mc-del { color: #d13438; width: 26px; height: 26px; border-radius: 6px; border: 1px solid var(--fluent-border); background: transparent; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; }
.mc-del:hover { background: rgba(209,52,56,0.08); }
.mc-hint { font-size: 11px; color: var(--fluent-text-soft); margin: 4px 0 8px; }
.mc-params { display: grid; grid-template-columns: repeat(6, 1fr); gap: 8px; }
.mc-field { display: flex; flex-direction: column; gap: 2px; }
.mc-field.span-2 { grid-column: span 2; }
.mc-field label { font-size: 11px; color: var(--fluent-text-soft); }
.mc-field .field { height: 30px; font-size: 12px; }
.mc-field .switch-row { display: flex; align-items: center; height: 30px; }
.mc-tip { grid-column: 1 / -1; font-size: 10px; color: var(--fluent-text-soft); }
.mc-effort { grid-column: 1 / -1; display: flex; align-items: center; gap: 8px; background: color-mix(in srgb, var(--accent, #0067c0) 8%, transparent); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; }
.mc-effort label { font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; }
.mc-effort-opts { display: flex; flex-wrap: wrap; gap: 6px; flex: 1; }
.effort-opt { display: inline-flex; align-items: center; justify-content: center; gap: 4px; font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 3px 10px; background: var(--fluent-card-bg); cursor: pointer; color: inherit; font-family: inherit; line-height: 1; appearance: none; -webkit-appearance: none; outline: none; }
.effort-opt:hover { border-color: var(--accent); }
.effort-opt:focus-visible { outline: none; box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 25%, transparent); }
.effort-opt.checked { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 12%, transparent); color: var(--accent); }
.effort-opt.checked:hover { border-color: var(--accent); }
.field { width: 100%; height: 32px; border-radius: 6px; border: 1px solid var(--fluent-border); background: var(--fluent-card-bg); padding: 0 10px; font-size: 13px; color: var(--fluent-text); }
.field:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent) 20%, transparent); }
.select-sm { min-width: 100px; height: 26px; font-size: 12px; }
.switch { position: relative; width: 40px; height: 20px; border-radius: 10px; background: rgba(0,0,0,0.18); border: none; cursor: pointer; flex-shrink: 0; }
.switch::after { content: ""; position: absolute; top: 4px; left: 4px; width: 12px; height: 12px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.3); transition: transform 0.2s; }
.switch.on { background: var(--accent); }
.switch.on::after { transform: translateX(20px); }
.mc-check { display: inline-flex; align-items: center; cursor: pointer; }
.mc-check input { position: absolute; opacity: 0; }
.check-mark { width: 18px; height: 18px; border: 1.5px solid #888; border-radius: 4px; display: inline-flex; align-items: center; justify-content: center; }
.mc-check input:checked + .check-mark { background: var(--accent); border-color: var(--accent); }
.mc-check input:checked + .check-mark::after { content: ""; width: 5px; height: 9px; border: solid #fff; border-width: 0 2px 2px 0; transform: rotate(45deg) translate(-1px, -1px); }
@media (max-width: 700px) { .mc-params { grid-template-columns: repeat(2, 1fr); } .mc-field.span-2 { grid-column: span 2; } }
</style>
