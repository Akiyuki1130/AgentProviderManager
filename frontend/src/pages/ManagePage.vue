<template>
  <div class="manage-page">
    <div class="manage-layout">
      <aside class="provider-sidebar fluent-card">
        <div class="sidebar-head">
          <h3>提供商</h3>
          <span class="sidebar-count">{{ filteredProviders.length }} / {{ providers.length }}</span>
        </div>
        <div class="sidebar-actions">
          <n-button size="small" type="primary" block @click="onAddProvider"><template #icon><n-icon :component="AddOutline" /></template>新增提供商</n-button>
        </div>
        <div class="sidebar-filter">
          <n-input v-model:value="providerQuery" size="small" clearable placeholder="搜索名称 / ID..." aria-label="搜索提供商" style="flex:1" @keyup.esc="providerQuery = ''" />
          <n-select v-model:value="providerSort" size="small" :options="providerSortOptions" style="width: 108px" />
        </div>
        <label class="sidebar-hide-builtin">
          <input type="checkbox" v-model="hideBuiltinNoKey" />
          <span>隐藏未填 API Key 的内置提供商</span>
        </label>
        <TransitionGroup name="list" tag="div" class="sidebar-list">
          <div v-if="filteredProviders.length === 0" key="empty" class="provider-empty">
            <n-icon :component="AddOutline" :size="22" />
            <span>暂无提供商</span>
            <n-button size="small" type="primary" ghost @click="onAddProvider">新增提供商</n-button>
          </div>
          <div
            v-for="p in filteredProviders"
            :key="p.id"
            class="provider-item"
            :class="{ selected: currentId === p.id }"
            @click="selectProvider(p.id)"
          >
            <div class="pi-top">
              <span class="pi-id">
                <template v-for="(seg, si) in highlightSegments(p.name || '', providerQuery)" :key="si">
                  <mark v-if="seg.hit">{{ seg.text }}</mark>
                  <template v-else>{{ seg.text }}</template>
                </template>
              </span>
              <!-- B6：仅当前打开的提供商挂脏点元素，isDirty 经 opacity 过渡显隐 -->
              <span v-if="currentId === p.id" class="dirty-dot" :class="{ on: isDirty }" :title="isDirty ? '有未保存的修改' : undefined" aria-hidden="true" />
            </div>
            <div class="pi-sub">{{ p.id }} · {{ p.model_count }} 个模型</div>
            <div class="pi-badges">
              <span class="badge">{{ p.kind }}</span>
              <span class="badge" :class="p.has_api_key ? 'badge-ok' : 'badge-warn'"><n-icon v-if="p.has_api_key" class="badge-icon" :component="CheckmarkOutline" :size="10" />{{ p.has_api_key ? 'API Key' : '无 Key' }}</span>
            </div>
          </div>
        </TransitionGroup>
      </aside>

      <div class="editor-pane">
        <div v-if="!currentProvider" class="editor-empty">
          <div class="empty-title">选择一个提供商进行编辑</div>
          <div class="empty-sub">在左侧选择提供商，或点击「新增提供商」创建</div>
        </div>

        <div v-else class="editor-body">
          <div class="fluent-card card-pad">
            <div class="card-title">提供商设置</div>
            <div class="editor-grid">
              <label class="form-label">Provider ID（修改即重命名）</label>
              <n-input v-model:value="editForm.id" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
              <label class="form-label">显示名称</label>
              <n-input v-model:value="editForm.name" placeholder="例如：我的供应商" />
              <label class="form-label">协议类型 (kind)</label>
              <n-select v-model:value="editForm.kind" :options="kindOptions" style="min-width: 200px" />
              <label class="form-label">Base URL</label>
              <n-input v-model:value="editForm.base_url" placeholder="https://api.example.com/v1" />
              <label class="form-label">API Key</label>
              <div class="kc-cell">
                <div style="display:flex; gap:8px; align-items:center">
                  <n-input v-model:value="editForm.api_key" :type="showKey ? 'text' : 'password'" placeholder="sk-..." style="flex:1" />
                  <label style="display:flex; gap:4px; align-items:center; font-size:12px; cursor:pointer"><input type="checkbox" v-model="showKey" /> 显示</label>
                  <n-button @click="showKeychainPicker = true"><template #icon><n-icon :component="CloudUploadOutline" /></template>从钥匙串</n-button>
                  <n-button :disabled="!editForm.api_key" @click="openSaveToKeychain"><template #icon><n-icon :component="DownloadOutline" /></template>存入钥匙串</n-button>
                </div>
                <div v-if="linkedEntry" class="keychain-chip">
                  <span class="badge" :class="keychainSynced ? 'badge-ok' : 'badge-warn'"><n-icon class="badge-icon" :component="KeyOutline" :size="10" />钥匙串·{{ linkedEntry.name }}{{ keychainSynced ? '' : '（不同步）' }}</span>
                  <n-button size="small" :disabled="keychainSynced" @click="onKeychainOverride">覆盖</n-button>
                  <n-button size="small" :disabled="keychainSynced" @click="onKeychainUpdate">更新</n-button>
                  <n-button size="small" type="error" ghost @click="onKeychainUnlink">解除</n-button>
                </div>
              </div>
              <label class="form-label">options 额外参数（JSON）</label>
              <n-input v-model:value="editForm.options_json" type="textarea" :rows="3" placeholder='{"headers": {...}}' />
            </div>
            <div class="editor-actions">
              <n-button size="small" type="primary" :loading="saving" title="保存提供商 (Ctrl+S)" @click="onSaveProvider"><template #icon><n-icon :component="SaveOutline" /></template>保存提供商</n-button>
              <n-button size="small" @click="onManualRefresh"><template #icon><n-icon :component="SyncOutline" /></template>手动重新获取</n-button>
              <n-button size="small" :loading="autoRefreshing" @click="onAutoRefresh"><template #icon><n-icon :component="SyncOutline" /></template>自动重新获取</n-button>
              <n-button size="small" :loading="probing" @click="onProbe"><template #icon><n-icon :component="FlashOutline" /></template>测试连接</n-button>
              <n-popconfirm @positive-click="confirmDeleteProvider">
                <template #trigger><n-button size="small" type="error" ghost><template #icon><n-icon :component="TrashOutline" /></template>删除提供商</n-button></template>
                确定删除提供商「{{ editForm.name || editForm.id }}」吗？
              </n-popconfirm>
              <label style="display:flex; gap:6px; align-items:center; font-size:12px; margin-left:auto"><input type="checkbox" v-model="editForm.api_key_required" /> 必须提供 API Key</label>
            </div>
            <!-- 自动保存状态行：安静地播报保存/暂停/失败，不弹 toast 打扰输入 -->
            <div v-if="autoSaveLabel" class="autosave-status" :class="autoSaveKind" role="status" aria-live="polite">
              <n-spin v-if="autoSave.status.value === 'saving'" :size="12" />
              <n-icon v-else class="autosave-icon" :component="statusIconFor(autoSaveKind)" :size="13" />
              <span>{{ autoSaveLabel }}</span>
            </div>
            <!-- 新建提供商尚未落盘：明确告知，避免用户以为已经保存 -->
            <div v-else-if="isUnsavedNew" class="autosave-status warn" role="status">
              <n-icon class="autosave-icon" :component="AlertCircleOutline" :size="13" />
              <span>{{ t('manage.unsavedNew', lang) }}</span>
            </div>
            <div v-if="probe.parts.length" class="probe-result" :class="probe.status" role="status" aria-live="polite">
              <n-icon class="probe-icon" :component="probeStatusIcon" :size="14" />
              <span v-for="(part, i) in probe.parts" :key="i" class="probe-part">
                <span v-if="i > 0" class="probe-sep">·</span>
                <span :class="{ tnum: part.num }">{{ part.text }}</span>
              </span>
            </div>
          </div>

          <div class="fluent-card card-pad list-wrap">
            <div class="list-header">
              <h3>模型列表</h3>
              <div style="display:flex; gap:12px; align-items:center">
                <n-input v-model:value="manageQuery" placeholder="搜索模型 ID / 名称..." aria-label="搜索模型" clearable style="width: 220px" size="small" @keyup.esc="manageQuery = ''" />
                <!-- 阶段 D：厂商多选筛选（选项由当前模型集动态生成），与搜索叠加生效 -->
                <n-select
                  v-model:value="vendorFilter"
                  :options="vendorOptions"
                  :placeholder="t('vendor.filterPlaceholder', lang)"
                  aria-label="按厂商筛选"
                  multiple
                  clearable
                  :max-tag-count="1"
                  style="width: 200px"
                  size="small"
                />
                <label class="vendor-group-toggle">
                  <span>{{ t('vendor.groupToggle', lang) }}</span>
                  <button class="switch" :class="{ on: vendorGroupOn }" role="switch" :aria-checked="vendorGroupOn" :aria-label="t('vendor.groupToggle', lang)" @click="vendorGroupOn = !vendorGroupOn" />
                </label>
                <span class="count-label">{{ filteredCards.length }} / {{ editForm.cards.length }}</span>
                <n-button size="small" @click="onAddModel"><template #icon><n-icon :component="AddOutline" /></template>手动添加模型</n-button>
              </div>
            </div>
            <div class="batch-bar">
              <div class="batch-row">
                <span class="label">批量编辑：</span>
                <n-button size="small" @click="batchSelectAll(true)">全选</n-button>
                <n-button size="small" @click="batchSelectAll(false)">全不选</n-button>
                <div class="batch-spacer" />
                <n-button size="small" type="error" ghost :disabled="!hasSelected" @click="onDeleteSelected"><template #icon><n-icon :component="TrashOutline" /></template>删除所选</n-button>
              </div>
              <div class="batch-row batch-row--wrap">
                <label class="batch-item">思考 <button class="switch" :class="{ on: batchReasoningOn }" role="switch" :aria-checked="batchReasoningOn" aria-label="批量切换思考" @click="batchToggleReasoning" /></label>
                <span class="batch-group-label">推理档位</span>
                <span v-for="v in variantOpts" :key="v" class="batch-pill" :class="{ unsupported: tierAllUnsupported(v) }" role="button" tabindex="0" @click="batchToggleVariant(v)" @keydown.enter.prevent="batchToggleVariant(v)" @keydown.space.prevent="batchToggleVariant(v)">{{ v }}</span>
                <!-- 阶段 F 修订：档位补全并入本行——它编辑的正是左边这组胶囊，放在同一行才看得出关系
                     （旧版把它单独放在批量栏最下面，与「推理档位」分成两处、两个名字）。
                     按钮文案与配色随方向开关变化：覆盖是破坏性动作，不能与并入长得一样。 -->
                <n-button
                  size="small"
                  :type="levelOverwrite ? 'error' : 'primary'"
                  ghost
                  :disabled="filteredCards.length === 0 || levelTargetCount === 0"
                  @click="onFillLevels"
                ><template #icon><n-icon :component="SparklesOutline" /></template>{{ levelOverwrite ? t('level.batchFillOverwrite', lang) : t('level.batchFill', lang) }}</n-button>
                <label class="check batch-item">
                  <input type="checkbox" v-model="levelOverwrite" :aria-label="t('level.batchOverwriteToggle', lang)" />
                  <span class="check-mark" />
                  <span>{{ t('level.batchOverwriteToggle', lang) }}</span>
                </label>
                <span class="batch-group-label">默认档位</span>
                <n-select v-model:value="batchDefault" :options="variantSelectOpts" placeholder="—" style="width: 100px" size="small" clearable @update:value="batchSetDefault" />
              </div>
              <!-- C3：说明批量作用域与灰色档位含义；套用不受支持档位时会先确认，不静默写入。
                   档位补全/覆盖的语义与计数同处一行：一个都处理不了时 summary 会直接说出原因。 -->
              <div class="batch-row batch-row--wrap">
                <span class="batch-hint">{{ t('reasoning.batchHint', lang) }}</span>
                <span class="batch-hint">{{ levelOverwrite ? t('level.batchHintOverwrite', lang) : t('level.batchHint', lang) }}</span>
                <span class="batch-hint">{{ levelSummary }}</span>
              </div>
              <div class="batch-row batch-row--wrap">
                <span class="batch-group-label">上下文长度</span>
                <n-input v-model:value="batchContext" placeholder="回车应用" size="small" style="width: 100px" @keydown.enter="applyBatchLimit('context')" />
                <span class="batch-group-label">输出长度</span>
                <n-input v-model:value="batchOutput" placeholder="回车应用" size="small" style="width: 100px" @keydown.enter="applyBatchLimit('output')" />
                <n-button size="small" type="primary" ghost :loading="autoMatchLoading" :disabled="filteredCards.length === 0" @click="onAutoMatchLimits">自动匹配</n-button>
                <span class="batch-hint">具体以提供商为准，自动数据仅供参考</span>
              </div>
              <!-- 阶段 E3：能力行——多选只列当前可见模型里「可补全」的能力位（带计数），
                   按钮按能力表补全空缺位（已有值一律不覆盖）。结果只落在卡片上，由既有保存载荷提交。 -->
              <div class="batch-row batch-row--wrap">
                <span class="batch-group-label">{{ t('cap.batchLabel', lang) }}</span>
                <n-select
                  v-model:value="capBits"
                  :options="capBitOptions"
                  :placeholder="t('cap.batchBitsPlaceholder', lang)"
                  :aria-label="t('cap.batchLabel', lang)"
                  multiple
                  clearable
                  :max-tag-count="1"
                  style="width: 200px"
                  size="small"
                />
                <n-button size="small" type="primary" ghost :disabled="filteredCards.length === 0" @click="onFillCapabilities"><template #icon><n-icon :component="SparklesOutline" /></template>{{ t('cap.batchFill', lang) }}</n-button>
                <span class="batch-hint">{{ t('cap.batchHint', lang) }}</span>
                <span class="batch-hint">{{ t('cap.batchArrayNote', lang) }}</span>
              </div>
            </div>
            <TransitionGroup name="list" tag="div" class="list-scroll">
              <div v-if="filteredCards.length === 0" key="empty" class="empty-state">
                <n-icon :component="CloudDownloadOutline" :size="26" />
                <div class="empty-title">尚无模型</div>
                <n-button size="small" type="primary" ghost :loading="autoRefreshing" @click="onAutoRefresh">从 API 获取模型</n-button>
              </div>
              <!-- 阶段 D：分组开启时按厂商分段渲染；分组头用 key 参与最外层 TransitionGroup，
                   卡片仍复用同一 ModelCard 组件。分组只改展示顺序/结构，批量作用域不变。 -->
              <template v-else-if="vendorGroupOn">
                <div v-for="g in groupedCards" :key="'grp-' + g.vendor" class="vendor-group">
                  <div class="vendor-group-head">
                    <span class="vendor-group-name">{{ vendorLabel(g.vendor) }}</span>
                    <span class="vendor-group-count">{{ t('vendor.groupCount', lang).replace('{n}', String(g.cards.length)) }}</span>
                  </div>
                  <ModelCard v-for="card in g.cards" :key="card.model_id" :card="card" :agent="settingStore.agent" :query="manageQuery" :show-select="true" @delete="removeCard(card)" @update="onCardUpdate" />
                </div>
              </template>
              <template v-else>
                <ModelCard v-for="card in filteredCards" :key="card.model_id" :card="card" :agent="settingStore.agent" :query="manageQuery" :show-select="true" @delete="removeCard(card)" @update="onCardUpdate" />
              </template>
            </TransitionGroup>
          </div>

          <!-- 脏态保存条：作为 .editor-body 最后一个子节点，sticky 相对真正滚动的 .editor-pane 生效 -->
          <StickySaveBar :visible="isDirty" :saving="saving" @save="onSaveProvider" @reset="discardChanges" />
        </div>
      </div>
    </div>

    <n-modal v-model:show="showAutoConfirm" preset="card" title="确认自动重新获取" style="width: 420px" :mask-closable="false">
      <div style="font-size: 13px; line-height: 1.6">
        该操作将会直接覆盖现有模型配置，是否确认继续？
      </div>
      <label style="display:flex; gap:6px; align-items:center; font-size:12px; margin-top:12px; cursor:pointer">
        <input type="checkbox" v-model="dontAskAgain" /> 不再提示
      </label>
      <template #footer>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <n-button @click="showAutoConfirm = false">取消</n-button>
          <n-button type="primary" :loading="autoRefreshing" @click="confirmAutoRefresh">确认覆盖</n-button>
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="showHttpConfirm" preset="card" title="安全提示" style="width: 420px" :mask-closable="false">
      <div style="font-size: 13px; line-height: 1.7">
        当前地址使用 <b class="warn-strong">http://</b> 明文协议连接。API Key 和请求内容将以明文传输，可能被网络窃听，存在安全风险。
        <br /><br />
        建议改用 <b>https://</b> 加密连接。若该地址仅用于本地开发或内网测试，可确认继续。
      </div>
      <template #footer>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <n-button @click="settleHttpConfirm(false)">{{ t('common.cancel', lang) }}</n-button>
          <n-button type="warning" @click="settleHttpConfirm(true)">仍要使用</n-button>
        </div>
      </template>
    </n-modal>

    <KeychainPicker v-model:show="showKeychainPicker" @pick="onKeychainPick" />

    <n-modal v-model:show="showKcSave" preset="card" title="存入钥匙串" style="width: 480px" :mask-closable="false">
      <div style="display:flex; flex-direction:column; gap:12px">
        <div>
          <label class="form-label">名称</label>
          <n-input v-model:value="kcSaveForm.name" placeholder="例如：我的供应商" />
        </div>
        <div>
          <label class="form-label">备注</label>
          <n-input v-model:value="kcSaveForm.note" placeholder="可选说明" />
        </div>
        <div>
          <label class="form-label">API Key</label>
          <n-input v-model:value="kcSaveForm.api_key" type="password" show-password-on="click" placeholder="sk-..." />
        </div>
        <div v-if="kcSaveError" class="modal-error">{{ kcSaveError }}</div>
      </div>
      <template #footer>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <n-button @click="showKcSave = false">取消</n-button>
          <n-button type="primary" :loading="kcSaving" @click="onSaveToKeychain"><template #icon><n-icon :component="SaveOutline" /></template>保存</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useSettingStore } from '../stores/setting'
import { registerManageGuard, requestLeave } from '../stores/leaveGuard'
import { NButton, NInput, NSelect, NPopconfirm, NModal, NIcon, NSpin } from 'naive-ui'
import {
  AddOutline, SaveOutline, TrashOutline, SyncOutline, FlashOutline,
  KeyOutline, CloudUploadOutline, DownloadOutline, CheckmarkOutline,
  CheckmarkCircleOutline, CloseCircleOutline, AlertCircleOutline, CloudDownloadOutline,
  SparklesOutline,
} from '@vicons/ionicons5'
import { uiPrompt, uiConfirm } from '../stores/uiDialog'
import { highlightSegments } from '../composables/useHighlight'
import { useVendorGroup, fetchVendorMeta } from '../composables/useVendorGroup'
import { useAutoSave } from '../composables/useAutoSave'
import { useCapabilityFill } from '../composables/useCapabilityFill'
import { useLevelFill } from '../composables/useLevelFill'
import ModelCard from '../components/ModelCard.vue'
import KeychainPicker from '../components/KeychainPicker.vue'
import StickySaveBar from '../components/ui/StickySaveBar.vue'
import type { ProviderSummary, ModelCard as Card, ProviderEdit, KeychainEntry, ProbeResult } from '../types'
import { REASONING_VARIANTS, REASONING_VARIANTS_BASIC, pickDefaultVariant } from '../constants/variants'
import { t } from '../i18n'
import * as api from '../api'

const router = useRouter()
const settingStore = useSettingStore()
const lang = computed(() => settingStore.resolvedLang)
const providers = ref<ProviderSummary[]>([])
const currentId = ref<string | null>(null)
const currentProvider = ref<ProviderEdit | null>(null)
const editForm = ref<ProviderEdit>({ id: '', name: '', kind: 'openai-compatible', base_url: '', api_key: '', api_key_required: false, options_json: '', cards: [] })
const savedProvider = ref<ProviderEdit | null>(null)
const showKey = ref(false)
const manageQuery = ref('')
const saving = ref(false)
// 是否已写入配置文件。新建的提供商在首次保存前为 false——「重新获取」等
// 需要它真实存在的操作据此先落盘，离开时也据此给出更重的丢弃警告。
const isPersisted = ref(false)
// 最近一次保存失败的后端原文，供自动保存状态行显示
const lastSaveError = ref('')
const hideBuiltinNoKey = ref(localStorage.getItem('zcpm.hideBuiltinNoKey') !== '0')
const variantOpts = REASONING_VARIANTS
const batchDefault = ref('')
const batchContext = ref('')
const batchOutput = ref('')
const autoMatchLoading = ref(false)
const showAutoConfirm = ref(false)
const dontAskAgain = ref(false)
const autoRefreshing = ref(false)
const showHttpConfirm = ref(false)
// 已就 http:// 明文确认过的 URL（trim+小写）。换 URL 会重新要求确认；
// 自动保存据此判断能否静默写入，绝不绕过明文确认门。
const httpAckUrl = ref('')
// 挂起中的 http 确认：resolve(true) 表示用户点了「仍要使用」
let httpConfirmResolve: ((ok: boolean) => void) | null = null
const providerQuery = ref('')
const providerSort = ref('default')
const providerSortOptions = [
  { label: '默认', value: 'default' },
  { label: '名称', value: 'name' },
  { label: '模型数', value: 'models' },
]
const probing = ref(false)
// 测试结果结构化：状态决定图标与色彩，parts 逐段渲染（num=true 段加 .tnum 等宽数字）
interface ProbePart { text: string; num?: boolean }
interface ProbeState { status: 'ok' | 'warn' | 'err' | ''; parts: ProbePart[] }
const probe = ref<ProbeState>({ status: '', parts: [] })
const probeStatusIcon = computed(() => probe.value.status === 'ok' ? CheckmarkCircleOutline : probe.value.status === 'warn' ? AlertCircleOutline : CloseCircleOutline)
const showKeychainPicker = ref(false)
const showKcSave = ref(false)
const kcSaving = ref(false)
const kcSaveError = ref('')
const kcSaveForm = reactive({ name: '', note: '', api_key: '' })
const kcLinks = ref<Record<string, string>>({})
// entryID -> 已读取的条目信息（含明文 Key），与 KeychainPage 的 secrets 缓存同一模式
const kcSecrets = reactive(new Map<string, { name: string; api_key: string; note: string }>())
const linkedEntry = ref<{ id: string; name: string; api_key: string; note: string } | null>(null)
const keychainSynced = computed(() => !!linkedEntry.value && linkedEntry.value.api_key === editForm.value.api_key)

const kindOptions = [
  { label: 'OpenAI 兼容 (openai-compatible)', value: 'openai-compatible' },
  { label: 'Anthropic 兼容 (anthropic)', value: 'anthropic' },
  { label: 'Responses 兼容 (responses)', value: 'responses' },
]
const variantSelectOpts = computed(() => variantOpts.map((v) => ({ label: v, value: v })))

const filteredProviders = computed(() => {
  let list = providers.value
  if (hideBuiltinNoKey.value) list = list.filter((p) => !(p.id.startsWith('builtin:') && !p.has_api_key))
  const q = providerQuery.value.trim().toLowerCase()
  if (q) list = list.filter((p) => (p.name || '').toLowerCase().includes(q) || (p.id || '').toLowerCase().includes(q))
  if (providerSort.value === 'name') return [...list].sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id, 'zh'))
  if (providerSort.value === 'models') return [...list].sort((a, b) => b.model_count - a.model_count)
  return list
})
// 阶段 D：厂商分组视图（共享 composable，与导入页同一实现；纯 APM 内部展示，绝不写盘）。
// 分组只影响渲染结构；filteredCards 仍是扁平数组，供既有批量逻辑复用。
// 厂商筛选属于「可见性」的一部分，因此参与 filteredCards（被筛掉的厂商不算可见）。
const { groupOn: vendorGroupOn, filter: vendorFilter, label: vendorLabel, matches: vendorMatches, options: vendorOptions, groupCards } = useVendorGroup(() => editForm.value.cards, lang)
const filteredCards = computed(() => {
  const q = manageQuery.value.trim().toLowerCase()
  if (!q) return editForm.value.cards.filter(vendorMatches)
  return editForm.value.cards.filter((c) => (((c.model_id || '') as string).toLowerCase().includes(q) || ((c.name || '') as string).toLowerCase().includes(q)) && vendorMatches(c))
})
// 按厂商分组后的渲染结构（保留组内既有顺序）。unknown 组始终排最后。
const groupedCards = computed(() => groupCards(filteredCards.value))
const allSelected = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.select))
const hasSelected = computed(() => filteredCards.value.some((c) => c.select))
const batchReasoningOn = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.reasoning))

watch(hideBuiltinNoKey, (v) => { localStorage.setItem('zcpm.hideBuiltinNoKey', v ? '1' : '0') })

// ---- 未保存更改检测 ----
function deepClone<T>(v: T): T { return JSON.parse(JSON.stringify(v)) as T }
function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (a === null || b === null) return a === b
  if (typeof a !== 'object' || typeof b !== 'object') return false
  const aArr = Array.isArray(a), bArr = Array.isArray(b)
  if (aArr !== bArr) return false
  if (aArr) {
    const aa = a as unknown[], bb = b as unknown[]
    if (aa.length !== bb.length) return false
    return aa.every((v, i) => deepEqual(v, bb[i]))
  }
  const ao = a as Record<string, unknown>, bo = b as Record<string, unknown>
  const ak = Object.keys(ao), bk = Object.keys(bo)
  if (ak.length !== bk.length) return false
  return ak.every((k) => Object.prototype.hasOwnProperty.call(bo, k) && deepEqual(ao[k], bo[k]))
}
// select 仅用于界面批量选择，不属于要保存的配置，比较时忽略
function normalizedProvider(p: ProviderEdit): unknown {
  return {
    id: p.id, name: p.name, kind: p.kind, base_url: p.base_url,
    api_key: p.api_key, api_key_required: p.api_key_required, options_json: p.options_json,
    cards: (p.cards || []).map((c) => {
      const o: Record<string, unknown> = {}
      for (const k of Object.keys(c)) if (k !== 'select') o[k] = (c as unknown as Record<string, unknown>)[k]
      return o
    }),
  }
}
// 未保存更改 = 还没落盘（新建）或与已保存基线不一致。二者必须分开表达：
// 新建的提供商磁盘上还不存在，任何守卫都不能把它当成“已保存”。
const isDirty = computed(() => {
  if (!currentProvider.value) return false
  if (!isPersisted.value) return true
  const base = savedProvider.value
  if (!base) return true
  return !deepEqual(normalizedProvider(editForm.value), normalizedProvider(base))
})

function discardChanges() {
  if (savedProvider.value) editForm.value = deepClone(savedProvider.value)
}

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

async function loadProviders() {
  const res = await api.ListProviders() as Record<string, unknown>
  if (res['success']) providers.value = (res['providers'] as ProviderSummary[]) || []
}
function selectProvider(id: string) {
  if (id === currentId.value) return
  void requestLeave(() => { void loadProvider(id) })
}

async function loadProvider(id: string) {
  // 明文确认是「针对某个 URL」的一次性放行：换成另一个提供商即失效，
  // 避免把 A 的放行顺延给 B 的 http 地址。（同 id 重新载入不清，见下）
  if (id !== currentId.value) httpAckUrl.value = ''
  currentId.value = id
  probe.value = { status: '', parts: [] }
  const res = await api.GetProvider(id) as Record<string, unknown>
  if (res['success']) {
    const raw = res['provider'] as ProviderEdit
    // 后端在零模型时可能返回 cards: null，在此边界归一化，保证进入响应式状态的 cards 始终是数组
    const p: ProviderEdit = { ...raw, cards: raw.cards || [] }
    currentProvider.value = p
    editForm.value = { ...p, cards: p.cards.map((c) => ({ ...c })) }
    savedProvider.value = deepClone(p)
    // 从磁盘读到的提供商必然已落盘
    isPersisted.value = true
    lastSaveError.value = ''
    await refreshLinkedEntry()
  } else toast('error', '加载失败', res['error'] as string)
}

async function onAddProvider() {
  // uiPrompt 取消或空输入解析为 null，早退语义与原 `if (!name) return` 一致
  const name = await uiPrompt({
    title: '新增提供商',
    label: 'Provider 显示名称（ID 将自动生成，可稍后修改）',
    placeholder: '例如：我的供应商',
    initialValue: '我的供应商',
    confirmText: '创建',
  })
  if (!name) return
  void requestLeave(() => { void createProvider(name) })
}

async function createProvider(name: string) {
  const id = await api.NewProviderID() as string
  editForm.value = { id, name: name || id, kind: 'openai-compatible', base_url: '', api_key: '', api_key_required: false, options_json: '', cards: [] }
  currentProvider.value = { ...editForm.value }
  currentId.value = id
  savedProvider.value = deepClone(editForm.value)
  // 尚未写入配置文件：savedProvider 只是「当前基线」，不代表磁盘上已有该提供商。
  // isPersisted=false 让脏判定、离开守卫与「重新获取」前的落盘都按未保存处理。
  isPersisted.value = false
  lastSaveError.value = ''
  probe.value = { status: '', parts: [] }
  linkedEntry.value = null
}

function isHttpUrl(url: string): boolean {
  return url.trim().toLowerCase().startsWith('http://')
}

// http:// 明文确认门：返回用户是否确认继续。同一 URL 确认过一次后本会话不再重复询问。
function confirmHttp(): Promise<boolean> {
  const url = editForm.value.base_url.trim().toLowerCase()
  if (httpAckUrl.value === url) return Promise.resolve(true)
  return new Promise<boolean>((resolve) => {
    httpConfirmResolve = resolve
    showHttpConfirm.value = true
  })
}

function settleHttpConfirm(ok: boolean) {
  const r = httpConfirmResolve
  httpConfirmResolve = null
  showHttpConfirm.value = false
  if (ok) httpAckUrl.value = editForm.value.base_url.trim().toLowerCase()
  r?.(ok)
}

function onSaveProvider() {
  void saveProvider()
}

// 保存当前表单。auto=true 表示这是后台自动保存（写盘时标记 op=auto_save，
// 让还原点清理把这条低价值记录优先淘汰）。
async function saveProvider(auto = false): Promise<boolean> {
  if (isHttpUrl(editForm.value.base_url)) {
    // 自动保存绝不绕过明文确认门：未确认时直接放弃本次自动写入，交给显式保存路径去问。
    if (auto) {
      if (httpAckUrl.value !== editForm.value.base_url.trim().toLowerCase()) return false
    } else if (!(await confirmHttp())) {
      return false
    }
  }
  // 本次要写入的内容快照：保存期间若用户又改了表单，用它区分“有没有新输入”。
  const written = deepClone(normalizedProvider(editForm.value))
  saving.value = true
  try {
    const payload: Record<string, unknown> = {
      id: editForm.value.id,
      name: editForm.value.name,
      kind: editForm.value.kind,
      base_url: editForm.value.base_url,
      api_key: editForm.value.api_key,
      api_key_required: editForm.value.api_key_required,
      options_json: editForm.value.options_json,
      cards: editForm.value.cards,
      _raw_provider: currentProvider.value?._raw_provider,
    }
    if (auto) payload['_operation'] = 'auto_save'
    const res = await api.SaveProvider(currentId.value || editForm.value.id, payload) as Record<string, unknown>
    if (res['success']) {
      currentId.value = res['provider_id'] as string
      isPersisted.value = true
      // 先刷新再提示成功，避免用户同时看到成功与失败提示；刷新失败也只报错、不影响保存结果
      try {
        await loadProviders()
        if (auto) await adoptSavedBaseline(written)
        else if (currentId.value) await loadProvider(currentId.value)
        lastSaveError.value = ''
        if (!auto) toast('success', '保存成功', `已保存 ${res['count']} 个模型到提供商 ${res['provider_id']}`)
      } catch (e) {
        if (!auto) toast('error', '加载失败', String(e))
      }
      return true
    } else {
      lastSaveError.value = String(res['error'] || '')
      if (!auto) toast('error', '保存失败', res['error'] as string)
      return false
    }
  } catch (e) {
    lastSaveError.value = String(e)
    if (!auto) toast('error', '保存失败', String(e))
    return false
  } finally { saving.value = false }
}

// 自动保存成功后的收尾：以磁盘结果作为新基线（后端可能规范化过字段，不更新基线
// 会让脏判定永远为真、反复触发自动保存）；若保存期间用户没有新输入才把磁盘版本
// 回写到表单，否则保留用户正在编辑的内容，交给调度器再落一次。
async function adoptSavedBaseline(written: unknown): Promise<void> {
  if (!currentId.value) return
  const res = await api.GetProvider(currentId.value) as Record<string, unknown>
  if (!res['success']) return
  const raw = res['provider'] as ProviderEdit
  const disk: ProviderEdit = { ...raw, cards: raw.cards || [] }
  savedProvider.value = deepClone(disk)
  // 后端对未建模字段的保留结果始终以磁盘为准，避免下次保存丢掉它们。
  currentProvider.value = { ...(currentProvider.value as ProviderEdit), _raw_provider: disk._raw_provider }
  if (deepEqual(normalizedProvider(editForm.value), written)) {
    currentProvider.value = disk
    editForm.value = { ...disk, cards: disk.cards.map((c) => ({ ...c })) }
  }
}

// 需要提供商真实存在时调用：尚未落盘就先把当前表单写进配置文件。
// 保存失败时返回 false（那时会显示真实原因，例如「请填写 Base URL」）。
async function ensurePersisted(): Promise<boolean> {
  if (isPersisted.value) return true
  return await saveProvider(false)
}

// 删除本身已有 n-popconfirm 确认层，且未保存的改动随之作废——不应再套「保存/不保存」守卫
// （否则删一个还没落盘的新提供商会被问「是否保存」）。
function confirmDeleteProvider() {
  void deleteProvider()
}

async function deleteProvider() {
  // 尚未落盘的新提供商磁盘上不存在，本地清空即可，不必也不该调删除接口。
  if (!isPersisted.value) {
    toast('success', '已删除', '已丢弃尚未保存的提供商')
    resetEditor()
    await loadProviders()
    return
  }
  const id = currentId.value!
  const res = await api.DeleteProvider(id) as Record<string, unknown>
  if (res['success']) { toast('success', '已删除', `提供商「${id}」已删除`); resetEditor(); await loadProviders() }
  else toast('error', '删除失败', res['error'] as string)
}

// 清空编辑区（删除、切换 Agent 后共用），并复位与「已落盘」相关的状态。
function resetEditor() {
  currentId.value = null
  currentProvider.value = null
  savedProvider.value = null
  editForm.value = { id: '', name: '', kind: 'openai-compatible', base_url: '', api_key: '', api_key_required: false, options_json: '', cards: [] }
  isPersisted.value = false
  httpAckUrl.value = ''
  lastSaveError.value = ''
  probe.value = { status: '', parts: [] }
  linkedEntry.value = null
}
async function onAddModel() {
  const mid = await uiPrompt({
    title: '手动添加模型',
    label: '模型 ID（即发送给 API 的模型名）',
    placeholder: '例如：gpt-4o-mini',
    initialValue: 'gpt-4o-mini',
    confirmText: '添加',
    validate: (v) => (editForm.value.cards.some((c) => c.model_id === v.trim()) ? '该模型 ID 已存在' : null),
  })
  if (!mid || !mid.trim()) return
  if (editForm.value.cards.some((c) => c.model_id === mid.trim())) { toast('info', '提示', `模型「${mid.trim()}」已在列表中。`); return }
  const card: Card = { model_id: mid.trim(), name: mid.trim(), reasoning: false, variants: [], default_variant: '', context: '', output: '', select: false }
  // 补一次能力表推断字段（厂商 / 支持档位），使分组与档位弱化对新增卡片立刻生效
  const meta = await fetchVendorMeta(card.model_id)
  if (meta) { card._vendor = meta._vendor; card._supported_levels = meta._supported_levels }
  editForm.value.cards.push(card)
}

function removeCard(card: Card) {
  const idx = editForm.value.cards.indexOf(card)
  if (idx >= 0) editForm.value.cards.splice(idx, 1)
}

function onCardUpdate() { /* reactive */ }

// C3：当前可见模型是否「全部」不支持某档位（列表非空时才判定，避免空列表误标灰）
function tierAllUnsupported(tier: string): boolean {
  const list = filteredCards.value
  return list.length > 0 && list.every((c) => !cardSupportsTier(c, tier))
}
function batchSelectAll(v: boolean) { for (const c of filteredCards.value) c.select = v }
function batchToggleReasoning() {
  const next = !batchReasoningOn.value
  if (next) {
    const onVariants = settingStore.reasoningAllIntensities ? [...REASONING_VARIANTS] : [...REASONING_VARIANTS_BASIC]
    for (const c of filteredCards.value) {
      c.reasoning = true
      c._raw_reasoning_enabled = true
      // 集合不含非 off 档位（空或仅 off）时补全档位；否则保持用户已有集合
      const cur = c.variants || []
      if (!cur.some((v) => v !== 'off')) {
        c.variants = [...onVariants]
        // 默认档位按集合内容挑选（不写死 high），避免集合不含 high 时制造矛盾数据
        c.default_variant = pickDefaultVariant(c.variants)
      }
    }
    return
  }
  // S1：关闭思考 = 档位集合收敛为 ["off"]；集合不含 off 词的模型无法表达关闭，跳过并计数提示。
  let skipped = 0
  for (const c of filteredCards.value) {
    const cur = c.variants || []
    if (cur.length > 0 && !cur.includes('off')) { skipped++; continue }
    c.reasoning = false
    c._raw_reasoning_enabled = false
    c.variants = ['off']
  }
  if (skipped > 0) toast('info', t('reasoning.batchSkipTitle', lang.value), t('reasoning.batchSkipBody', lang.value).replace('{n}', String(skipped)))
}
// C3：某档位对某模型是否「被支持」＝_supported_levels 存在（非空）且包含它；
// 未推断到的模型（空/未定义）一律按支持处理，避免误报。自填档位不在内建域内也按支持处理。
function cardSupportsTier(c: Card, tier: string): boolean {
  const lv = c._supported_levels
  if (!lv || lv.length === 0) return true
  if (!(REASONING_VARIANTS as readonly string[]).includes(tier)) return true
  return lv.includes(tier)
}
// C3：批量套用某档位前，统计当前可见模型中不支持它的数量；有则先确认（不静默写入）。
async function confirmUnsupportedTier(tier: string): Promise<boolean> {
  const unsupported = filteredCards.value.filter((c) => !cardSupportsTier(c, tier)).length
  if (unsupported === 0) return true
  return await uiConfirm({
    content: t('reasoning.batchUnsupported', lang.value).replace('{n}', String(unsupported)).replace('{tier}', tier),
    positiveText: t('common.ok', lang.value),
  })
}
async function batchToggleVariant(v: string) {
  // 仅「新增档位」需要确认；移除已选档位不涉及写入不支持项，直接执行。
  const willAdd = filteredCards.value.some((c) => !(c.variants || []).includes(v))
  if (willAdd && !(await confirmUnsupportedTier(v))) return
  for (const c of filteredCards.value) {
    const arr = c.variants || []
    const idx = arr.indexOf(v)
    if (idx >= 0) arr.splice(idx, 1)
    else { arr.push(v); c.reasoning = true; c._raw_reasoning_enabled = true }
    c.variants = [...arr]
    if (c.default_variant && !arr.includes(c.default_variant)) c.default_variant = arr[0] || ''
  }
}
async function batchSetDefault(v: string) {
  if (!v) return
  if (!(await confirmUnsupportedTier(v))) return
  for (const c of filteredCards.value) {
    c.reasoning = true
    c._raw_reasoning_enabled = true
    // 后端返回的模型 variants 可能为 null，需要兜底
    const arr = c.variants || []
    if (!arr.includes(v)) arr.push(v)
    c.variants = [...arr]
    c.default_variant = v
  }
  batchDefault.value = ''
}
function applyBatchLimit(field: 'context' | 'output') {
  const val = field === 'context' ? batchContext.value : batchOutput.value
  for (const c of filteredCards.value) (c as Record<string, unknown>)[field] = val
}

// ---- 阶段 E3：能力批量行 ----
// 选项生成 / 计数 / 只补空缺 / 失效选择收敛 / 汇报都在 useCapabilityFill（两页同一实现）；
// 作用域与其它批量行一致：当前筛选出的可见模型。结果只落在卡片的 capabilities 上，
// 由两页既有的卡片载荷（保存 / 导入）提交，不新增接口、不直接写盘。
const { picked: capBits, options: capBitOptions, fill: onFillCapabilities } = useCapabilityFill(() => filteredCards.value, lang)

// ---- 阶段 F 修订：档位并入「推理档位」行 ----
// 计数 / 并入 / 覆盖 / 确认 / 汇报都在 useLevelFill（两页同一实现）；作用域与其它批量行一致：
// 当前筛选出的可见模型。结果只落在卡片的 variants / default_variant 上，由两页既有的卡片载荷
// （保存 / 导入）提交，不新增接口、不直接写盘。`overwrite` 是动作方向开关（默认并入、不持久化），
// `targetCount` 供按钮禁用，`summary` 是计数/原因的现成文案（逻辑只此一处）。
const {
  overwrite: levelOverwrite,
  targetCount: levelTargetCount,
  summary: levelSummary,
  fill: onFillLevels,
} = useLevelFill(() => filteredCards.value, lang)

async function onAutoMatchLimits() {
  const list = filteredCards.value
  if (list.length === 0) return
  const hasFilled = list.some((c) => isLimitFilled(c.context) || isLimitFilled(c.output))
  if (hasFilled && !(await uiConfirm({ content: '部分模型已填写上下文/输出长度，自动匹配会用预设值覆盖这些已填写的值，是否确认继续？', positiveText: '确认覆盖', danger: true }))) {
    return
  }
  autoMatchLoading.value = true
  try {
    const res = await api.ApplyModelPresetsForSite(list, editForm.value.base_url, editForm.value.kind)
    if (!res['success']) { toast('error', '自动匹配失败', (res['error'] as string) || ''); return }
    const updated = (res['cards'] as Card[]) || []
    const byId = new Map(updated.map((c) => [c.model_id, c]))
    for (let i = 0; i < editForm.value.cards.length; i++) {
      const m = byId.get(editForm.value.cards[i].model_id)
      if (m) editForm.value.cards[i] = m
    }
    toast('success', '自动匹配', `已为 ${res['filled'] ?? 0} 个模型填充上下文/输出长度`)
  } catch (e) {
    toast('error', '自动匹配失败', String(e))
  } finally {
    autoMatchLoading.value = false
  }
}

function isLimitFilled(v: string | number | undefined): boolean {
  if (v === undefined || v === null) return false
  if (typeof v === 'string') return v.trim() !== ''
  return v > 0
}
async function onDeleteSelected() {
  const selected = filteredCards.value.filter((c) => c.select)
  if (selected.length === 0) return
  if (!(await uiConfirm({ content: `确定删除选中的 ${selected.length} 个模型吗？`, positiveText: '删除', danger: true }))) return
  for (const c of selected) {
    const idx = editForm.value.cards.indexOf(c)
    if (idx >= 0) editForm.value.cards.splice(idx, 1)
  }
}

async function onManualRefresh() {
  if (!currentId.value || !currentProvider.value) { toast('error', '操作失败', '请先选择一个提供商'); return }
  // 导入页横幅会指向这个提供商，先把它落盘，取消时也不丢数据。
  if (!(await ensurePersisted())) return
  sessionStorage.setItem('zcode-pm:manualRefresh', JSON.stringify({
    providerId: editForm.value.id,
    providerName: editForm.value.name,
    baseUrl: editForm.value.base_url,
    apiKey: editForm.value.api_key,
    kind: editForm.value.kind,
  }))
  router.push('/import')
}

async function onAutoRefresh() {
  if (!currentId.value) return
  if (isHttpUrl(editForm.value.base_url)) {
    if (!(await confirmHttp())) return
  }
  if (localStorage.getItem('zcode-pm:skipAutoRefreshConfirm') === '1') {
    void doAutoRefresh()
    return
  }
  dontAskAgain.value = false
  showAutoConfirm.value = true
}

async function confirmAutoRefresh() {
  if (dontAskAgain.value) localStorage.setItem('zcode-pm:skipAutoRefreshConfirm', '1')
  showAutoConfirm.value = false
  await doAutoRefresh()
}

async function doAutoRefresh() {
  if (!currentId.value) return
  // 自动重新获取只是读接口、不写盘，但后端按 ID 查提供商；新建未落盘的先保存，
  // 否则会得到「提供商不存在」这种误导性报错。
  if (!(await ensurePersisted())) return
  autoRefreshing.value = true
  try {
    const res = await api.RefreshProviderModels(currentId.value, editForm.value.base_url, editForm.value.api_key) as Record<string, unknown>
    if (!res['success']) { toast('error', '获取失败', res['error'] as string); return }
    const models = (res['models'] as Card[]) || []
    toast('success', '获取成功', `成功获取 ${models.length} 个模型，已覆盖当前列表`)
    // 重新获取后默认全不选，避免误操作批量编辑/删除
    for (const c of models) c.select = false
    editForm.value.cards = models
  } finally {
    autoRefreshing.value = false
  }
}

// ---- 测试连接 ----
function formatProbe(res: ProbeResult): ProbeState {
  // 后端把「models 通、chat 挂」也记为 success:false，分支按段判定而非整体 success
  if (!res.models_ok) return { status: 'err', parts: [{ text: res.error || res.error_code || '模型接口不可用' }] }
  const models: ProbePart = { text: `模型 ${res.models_count ?? 0} 个 (${res.models_ms ?? '—'}ms)`, num: true }
  if (res.chat_skipped) return { status: 'ok', parts: [models, { text: 'chat 跳过（无可用模型列表）' }] }
  if (!res.chat_ok) return { status: 'warn', parts: [{ text: `chat 端点失败：${res.error || '未知错误'}` }] }
  return { status: 'ok', parts: [models, { text: `chat ${res.chat_ms ?? '—'}ms`, num: true }] }
}

async function onProbe() {
  if (!editForm.value.base_url.trim()) { probe.value = { status: 'warn', parts: [{ text: '请先填写 Base URL' }] }; return }
  probing.value = true
  try {
    // 已存在的提供商走 TestProviderConnection（表单值作为覆盖项）；新建未保存的退回 TestConnectionRaw
    const exists = providers.value.some((p) => p.id === currentId.value)
    const res = exists
      ? await api.TestProviderConnection(currentId.value || editForm.value.id, editForm.value.base_url, editForm.value.api_key)
      : await api.TestConnectionRaw(editForm.value.base_url, editForm.value.api_key, editForm.value.kind)
    probe.value = formatProbe(res)
  } catch (e) {
    probe.value = { status: 'err', parts: [{ text: String(e) }] }
  } finally { probing.value = false }
}

// ---- 钥匙串关联 ----
function agentLabel(agent: string): string {
  if (agent === 'deepseek') return 'DeepSeek Harness'
  return 'ZCode'
}
function linkKey(providerId: string): string { return `${settingStore.agent}|${providerId}` }

async function loadKeychainLinks() {
  try {
    const res = await api.GetKeychainLinks()
    kcLinks.value = res.success ? (res.links || {}) : {}
  } catch { kcLinks.value = {} }
}

async function getEntryInfo(entryId: string): Promise<{ name: string; api_key: string; note: string } | null> {
  const cached = kcSecrets.get(entryId)
  if (cached) return cached
  try {
    const res = await api.GetKeychainEntry(entryId)
    if (!res.success) return null
    const entry = res.entry as Record<string, unknown>
    const info = {
      name: (entry['name'] as string) || '',
      api_key: (entry['api_key'] as string) || (entry['apiKey'] as string) || '',
      note: (entry['note'] as string) || '',
    }
    kcSecrets.set(entryId, info)
    return info
  } catch { return null }
}

// 链接一律以「已保存的提供商 ID」为准（与 SaveProvider 的取法一致），
// 否则改名未保存时会把链接写到新 ID 上、chip 显示的却是旧 ID 的链接。
const linkTargetId = computed(() => currentId.value || editForm.value.id)

async function refreshLinkedEntry() {
  linkedEntry.value = null
  const id = linkTargetId.value
  if (!id) return
  const entryId = kcLinks.value[linkKey(id)]
  if (!entryId) return
  const info = await getEntryInfo(entryId)
  if (info) linkedEntry.value = { id: entryId, ...info }
}

async function onKeychainPick(e: KeychainEntry) {
  const info = await getEntryInfo(e.id)
  if (!info || !info.api_key) { toast('error', '填入失败', '无法读取该钥匙串条目'); return }
  editForm.value.api_key = info.api_key
  // 关联以提供商 ID 记录，新建未落盘时先保存，否则会关联到一个不存在的提供商上。
  if (!(await ensurePersisted())) return
  const res = await api.SetKeychainLink(linkTargetId.value, e.id)
  if (!res.success) { toast('error', '关联失败', res.error || ''); return }
  kcLinks.value[linkKey(linkTargetId.value)] = e.id
  linkedEntry.value = { id: e.id, ...info }
  toast('success', '已关联钥匙串', `已填入「${e.name}」并关联到当前提供商`)
}

function openSaveToKeychain() {
  kcSaveForm.name = editForm.value.name || editForm.value.id
  kcSaveForm.note = `${agentLabel(settingStore.agent)} 提供商`
  kcSaveForm.api_key = editForm.value.api_key
  kcSaveError.value = ''
  showKcSave.value = true
}

async function onSaveToKeychain() {
  kcSaveError.value = ''
  if (!kcSaveForm.name.trim()) { kcSaveError.value = '请填写名称'; return }
  if (!kcSaveForm.api_key.trim()) { kcSaveError.value = '请填写 API Key'; return }
  kcSaving.value = true
  try {
    const res = await api.SaveKeychainEntry({ id: '', name: kcSaveForm.name.trim(), api_key: kcSaveForm.api_key, note: kcSaveForm.note })
    if (!res.success) { kcSaveError.value = (res.error as string) || '保存失败'; return }
    const entryId = ((res.entry as Record<string, unknown> | undefined)?.['id'] as string) || ''
    const info = { name: kcSaveForm.name.trim(), api_key: kcSaveForm.api_key, note: kcSaveForm.note }
    if (entryId) {
      kcSecrets.set(entryId, info)
      // 关联写的是提供商 ID：新建未落盘时先保存，避免关联到不存在的提供商。
      if (!(await ensurePersisted())) { kcSaveError.value = lastSaveError.value || '提供商尚未保存'; return }
      const link = await api.SetKeychainLink(linkTargetId.value, entryId)
      if (link.success) {
        kcLinks.value[linkKey(linkTargetId.value)] = entryId
        linkedEntry.value = { id: entryId, ...info }
      }
    }
    showKcSave.value = false
    toast('success', '已存入钥匙串', `已保存「${info.name}」并关联到当前提供商`)
  } catch (e) {
    kcSaveError.value = String(e)
  } finally { kcSaving.value = false }
}

function onKeychainOverride() {
  if (linkedEntry.value) editForm.value.api_key = linkedEntry.value.api_key
}

async function onKeychainUpdate() {
  const e = linkedEntry.value
  if (!e) return
  const res = await api.SaveKeychainEntry({ id: e.id, name: e.name, api_key: editForm.value.api_key, note: e.note })
  if (!res.success) { toast('error', '更新失败', (res.error as string) || ''); return }
  const info = { name: e.name, api_key: editForm.value.api_key, note: e.note }
  kcSecrets.set(e.id, info)
  linkedEntry.value = { id: e.id, ...info }
  toast('success', '钥匙串已更新', '已把钥匙串条目更新为当前 API Key')
}

async function onKeychainUnlink() {
  const res = await api.RemoveKeychainLink(linkTargetId.value)
  if (!res.success) { toast('error', '解除失败', res.error || ''); return }
  delete kcLinks.value[linkKey(linkTargetId.value)]
  linkedEntry.value = null
  toast('success', '已解除关联', '已解除与钥匙串条目的关联')
}

// ---- 自动保存 ----
// 自动保存的预检：返回非 null 表示本轮不宜自动写入，值是给用户看的暂停原因。
// 只做「静默也无法成功」的检查——空 Base URL、非法 ID、未确认的 http 明文。
function autoSavePreflight(): string | null {
  const f = editForm.value
  if (!f || !currentProvider.value) return t('manage.reasonProviderId', lang.value)
  if (!f.id.trim() || !/^[a-zA-Z0-9_:\-]+$/.test(f.id.trim())) return t('manage.reasonProviderId', lang.value)
  if (!f.base_url.trim()) return t('manage.reasonBaseUrl', lang.value)
  if (isHttpUrl(f.base_url) && httpAckUrl.value !== f.base_url.trim().toLowerCase()) return t('manage.reasonHttp', lang.value)
  return null
}

const autoSave = useAutoSave({
  isDirty: () => isDirty.value,
  enabled: () => settingStore.autoSave,
  preflight: autoSavePreflight,
  save: async () => {
    const ok = await saveProvider(true)
    return { ok, error: ok ? undefined : lastSaveError.value }
  },
})

// 自动保存状态行文案：把调度器状态翻成一行安静的提示，不弹 toast 打扰输入。
const autoSaveLabel = computed(() => {
  if (!settingStore.autoSave) return ''
  const st = autoSave.status.value
  if (st === 'saving') return t('manage.autosaveSaving', lang.value)
  if (st === 'saved') return t('manage.autosaveSaved', lang.value)
  if (st === 'paused') return t('manage.autosavePaused', lang.value).replace('{reason}', autoSave.pausedReason.value || '')
  if (st === 'error') return t('manage.autosaveFailed', lang.value).replace('{error}', autoSave.lastError.value || '')
  return ''
})
const autoSaveKind = computed(() => {
  const st = autoSave.status.value
  if (st === 'error') return 'err'
  if (st === 'paused') return 'warn'
  if (st === 'saved') return 'ok'
  return ''
})
// 新建未落盘时持续提示一次，让用户知道这个提供商还没进配置文件。
const isUnsavedNew = computed(() => !!currentProvider.value && !isPersisted.value)

// 自动保存状态行图标：按语义档位取图标，状态字符不进文本
function statusIconFor(kind: string) {
  if (kind === 'ok') return CheckmarkCircleOutline
  if (kind === 'warn') return AlertCircleOutline
  if (kind === 'err') return CloseCircleOutline
  return AlertCircleOutline
}

function onAgentChanged() {
  resetEditor()
  void loadProviders()
  void loadKeychainLinks()
}
// B7：Ctrl+S（或 Cmd+S）拦截浏览器保存对话框；仅在脏态复用 onSaveProvider 保存入口（保留 http 明文确认门）
function onGlobalKeydown(e: KeyboardEvent) {
  if (!(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== 's') return
  e.preventDefault()
  if (!e.repeat && isDirty.value && !saving.value) onSaveProvider()
}
onMounted(() => {
  // 自动保存的 flush 与调度器共用单飞锁，因此优先用它；autoFlush 决定能否「静默落盘、不弹窗」。
  registerManageGuard({
    isDirty: () => isDirty.value,
    isUnsavedNew: () => isUnsavedNew.value,
    save: () => saveProvider(false),
    discard: discardChanges,
    flush: () => autoSave.flush(),
    // flush 内部会 await 在途写入并做单飞，因此只需判断自动保存是否开启。
    autoFlush: () => settingStore.autoSave,
  })
  autoSave.start()
  void loadProviders()
  void loadKeychainLinks()
  window.addEventListener('agent-changed', onAgentChanged)
  window.addEventListener('keydown', onGlobalKeydown)
})
onUnmounted(() => {
  autoSave.stop()
  registerManageGuard(null)
  window.removeEventListener('agent-changed', onAgentChanged)
  window.removeEventListener('keydown', onGlobalKeydown)
})
</script>

<style scoped>
.manage-page { display: flex; flex-direction: column; flex: 1; min-height: 0; height: 100%; }
.manage-layout { display: grid; grid-template-columns: 244px 1fr; gap: 12px; flex: 1; min-height: 0; height: 100%; overflow: hidden; }
.provider-sidebar { display: flex; flex-direction: column; padding: 16px 16px 16px; min-height: 0; height: 100%; overflow: hidden; align-self: stretch; }
.sidebar-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.sidebar-head h3 { font-size: 14px; font-weight: 600; margin: 0; }
.sidebar-count { font-size: 12px; color: var(--fluent-text-soft); background: var(--fluent-border); border-radius: 999px; padding: 1px 8px; }
.sidebar-actions { margin-bottom: 8px; }
.sidebar-filter { display: flex; gap: 8px; align-items: center; margin-bottom: 8px; }
.sidebar-hide-builtin { display: flex; gap: 6px; align-items: center; font-size: 11px; color: var(--fluent-text-soft); margin-bottom: 8px; cursor: pointer; }
.provider-empty { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 16px; text-align: center; font-size: 12px; color: var(--fluent-text-soft); }
/* position:relative 给 .list-leave-active 的绝对定位提供锚点，删除补位时离场项不脱离列表 */
.sidebar-list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 8px; min-height: 0; position: relative; }
.provider-item { background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 10px; cursor: pointer; }
.provider-item.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
.pi-top { display: flex; align-items: center; gap: 8px; }
.pi-id { font-size: 13px; font-weight: 600; flex: 1; word-break: break-all; }
/* B6 脏点：仅在当前打开的条目上存在，isDirty 驱动 opacity 显隐（0.2s） */
.dirty-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--warn); flex-shrink: 0; align-self: center; opacity: 0; transition: opacity var(--dur-standard, 0.2s) var(--ease-standard, cubic-bezier(0.33, 0, 0, 1)); }
.dirty-dot.on { opacity: 1; }
.pi-sub { font-size: 11px; color: var(--fluent-text-soft); margin-top: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pi-badges { display: flex; gap: 4px; margin-top: 6px; flex-wrap: nowrap; min-width: 0; }
/* 侧栏紧凑徽标变体（10px/16px 行高/nowrap，与全局 .badge 确有差异故保留 scoped）；色档 badge-ok/warn 用全局定义 */
.badge { display: inline-flex; align-items: center; gap: 3px; font-size: 10px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 0 6px; line-height: 16px; white-space: nowrap; }
.badge-icon { flex-shrink: 0; }
.editor-pane { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; height: 100%; gap: 12px; overflow-y: auto; overscroll-behavior: contain; padding-right: 2px; }
.editor-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; flex: 1; gap: 8px; text-align: center; color: var(--fluent-text-soft); background: var(--fluent-card-bg); border: 1px dashed var(--fluent-border); border-radius: 12px; padding: 32px; }
/* .empty-title/.empty-sub 单一定义见下方（原此处 16px 重复定义被后文覆盖，实际渲染一直是 14px，行为不变） */
.editor-body { display: flex; flex-direction: column; gap: 12px; }
/* 容器圆角 12px 对齐全局 .fluent-card 档位；仅保留本页去阴影/描边的扁平化差异 */
.fluent-card { background: var(--fluent-card-bg); border-radius: 12px; box-shadow: none; border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.editor-grid { display: grid; grid-template-columns: minmax(0, auto) minmax(0, 1fr); gap: 8px 12px; align-items: center; }
.editor-grid > * { min-width: 0; }
.editor-grid :deep(.n-input), .editor-grid :deep(.n-select) { width: 100%; max-width: 100%; min-width: 0; }
.form-label { min-width: 0; font-size: 12px; color: var(--fluent-text-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.editor-actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 12px; align-items: center; overflow: visible; }
.editor-actions > * { flex-shrink: 0; }
.kc-cell { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.keychain-chip { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.modal-error { color: var(--err); font-size: 12px; }
.warn-strong { color: var(--warn); }
/* 自动保存状态行：图标 + 语义色 + 安静基色，常驻于动作区下方 */
.autosave-status { display: flex; align-items: center; gap: 6px; margin-top: 8px; font-size: 12px; color: var(--fluent-text-soft); }
.autosave-status .autosave-icon { flex-shrink: 0; }
.autosave-status.ok { color: var(--ok); }
.autosave-status.warn { color: var(--warn); }
.autosave-status.err { color: var(--err); }
/* 测试结果：图标 + 语义色 + 分段文本（数字段挂全局 .tnum 等宽） */
.probe-result { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px; font-size: 12px; color: var(--fluent-text-soft); margin-top: 8px; word-break: break-all; }
.probe-result .probe-icon { align-self: center; flex-shrink: 0; }
.probe-part { display: inline-flex; align-items: baseline; gap: 6px; }
.probe-sep { opacity: 0.55; }
.probe-result.ok { color: var(--ok); }
.probe-result.err { color: var(--err); }
.probe-result.warn { color: var(--warn); }

.list-wrap { display: flex; flex-direction: column; gap: 8px; }
.list-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; }
.list-header h3 { font-size: 14px; font-weight: 600; }
.count-label { font-size: 12px; color: var(--fluent-text-soft); }
/* 阶段 D 厂商分组/开关的外观定义在 global.css 控件规范层（管理页与导入页共用） */
.batch-bar { display: flex; flex-direction: column; gap: 8px; padding: 8px 12px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; }
.batch-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.batch-row--wrap { align-items: center; }
.batch-spacer { flex: 1; }
.batch-bar .label { font-size: 12px; font-weight: 600; }
.batch-item { display: inline-flex; gap: 4px; align-items: center; font-size: 11px; }
.batch-group-label { font-size: 11px; color: var(--fluent-text-soft); font-weight: 600; }
.batch-hint { font-size: 10px; color: var(--fluent-text-soft); opacity: 0.75; }
.batch-pill { font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 2px 8px; cursor: pointer; }
.batch-pill:hover { border-color: var(--accent); }
/* C3：当前可见模型均不支持的档位弱化（虚框 + 低饱和），与 ModelCard 的 .pill.unsupported 同语义 */
.batch-pill.unsupported { border-style: dashed; color: var(--fluent-text-soft); opacity: 0.7; }
.batch-pill.unsupported:hover { opacity: 1; }
.list-scroll { display: flex; flex-direction: column; min-height: 0; position: relative; }
@media (max-width: 1099px) { .manage-layout { grid-template-columns: 1fr; } .provider-sidebar { height: auto; max-height: 32vh; } }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 24px; color: var(--fluent-text-soft); text-align: center; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
/* 原 scoped .switch 重复定义已删除，统一走 global.css 控件规范层（顺带修复 dark 轨道 rgba(0,0,0,.18) 隐形问题，D2/B10） */
</style>
