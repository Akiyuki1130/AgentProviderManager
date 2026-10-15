# AgentProviderManager 前端 UI 指引

> 本文是 APM 前端的**长期约定**，不是一次性整改记录。实施过程与逐条差距清单见（已完成归档的）[`Archive/UIUX-POLISH-PLAN.md`](Archive/UIUX-POLISH-PLAN.md)。
> 适用范围：`frontend/src/**`。所有改动（含 AI 子代理）在提交前都应对照本文件自查。

---

## 1. 为什么有这份文件

APM 早期移植了 Fluent 骨架（CSS 变量双主题、accent、侧栏指示条、路由转场、`fluent-card`），但精致度是系统性缺失的，病根有四层，2026-10 的一轮整改逐条对症：

| 病根 | 具体表现 | 现状 |
|---|---|---|
| Emoji 当图标 | 40+ 处 emoji / `✔✕⚠` 状态字符混在按钮、弹窗标题、徽标与 i18n 字典里；不跟 accent、基线不齐、dark 刺眼、尺寸不可控 | **已清零**，全部改 `@vicons/ionicons5` |
| 原生 OS 弹窗 | `prompt()` / `confirm()` 共 8 处，主题 / 圆角 / 字体 / 动效全断裂 | **已清零**，改主题化 dialog host |
| 自制控件缺精致基因 | `.switch` 轨道在深底隐形、`.check-mark` 边框写死 `#888`、语义色无体系 | 收敛进 `global.css` 单一出处 |
| 缺少秩序感 | 无统一页头、无错峰入场、状态硬切、时长 / 圆角 / 尺寸各页随意 | token 化 + 复用组件落地 |

**这份文件的存在意义就是让上面四类问题不复发。** 新增页面 / 组件时，先读本文再动手。

---

## 2. 硬性红线（违反即返工）

1. **零 emoji。** UI 里任何地方（模板、文案、toast、徽标、按钮、标题）都不得出现 emoji 或 `✔✕⚠✓✗` 之类状态字符。图标一律用依赖 `@vicons/ionicons5`，经 naive-ui `<n-icon>` 渲染；按钮图标走 `#icon` 插槽。
   - 尺寸约定：**按钮内图标 16–18px**（页面头图标 18px / 徽标内小图标 10–11px / 提示型图标 14px / 空态大图标 22–26px）。
   - 自查：`frontend/src` 里 emoji 码点扫描结果必须为 0。
   - 注意：**emoji 不进 i18n 字典**。字典只存文字，图标在模板层加。历史遗留的 `📥 xxx` 这类前缀要清掉，而不是留在文案里。
2. **零原生弹窗。** 禁止 `window.prompt()` / `window.confirm()` / `window.alert()`。用 `stores/uiDialog.ts` 的 `uiConfirm()` / `uiPrompt()`（宿主 `components/ui/UiDialogHost.vue` 已在 `App.vue` 挂载一次）。
   - 删除类操作：`uiConfirm({ danger: true })` 或就近 `n-popconfirm`。
   - 跨行 / 跨文件影响类：`uiConfirm`（可带 `detail` 显示路径与影响范围）。
   - 需要输入：`uiPrompt`，支持 `validate` 内联校验 + Enter 提交。
3. **不引入新的 UI 库。** 现有栈已足够：`naive-ui` + `@vicons/ionicons5` + 自绘 token 化控件。不要为了一个效果加依赖。
4. **不推翻布局骨架。** 「侧栏 + 顶栏 + 内容区」结构与路由组织保持稳定；改动是精致的增量，不是重写。
5. **颜色 / 尺寸 / 时长必须走 token。** 禁止在页面里写死语义色（`#107c10` / `#d13438` / `#ca5010` / `#c42b1c`）或硬编码动画时长；见第 3 节。

---

## 3. 设计 token（单一出处：`styles/global.css` + `styles/theme.ts`）

### 3.1 颜色

| token | light | dark | 用途 |
|---|---|---|---|
| `--fluent-bg` | `#f3f3f3` | `#202020` | 页面底色 |
| `--fluent-card-bg` | `#ffffff` | `#2b2b2b` | 卡片底色 |
| `--fluent-text` / `--fluent-text-soft` | 黑 90% / 60% | 白 90% / 60% | 主 / 次文本 |
| `--ok` / `--warn` / `--err` | `#107c10` / `#ca5010` / `#c42b1c` | `#6ccb5f` / `#ffb160` / `#ff6e6e` | 语义色（dark 提亮降饱和） |
| `--ok-soft` / `--warn-soft` / `--err-soft` | 语义色 12~14% `color-mix` | 同 | 徽标 / 状态底色 |
| `--accent-text-strong` | `var(--accent)` | accent 与白 45% 混合 | 文本态强调色（浅底上 accent 太暗） |

**三条与主题相关的硬约束：**

- 底色 / 卡片色在 **`global.css`、`theme.ts`（dark 的 `bodyColor` / `cardColor`）、`index.html` 内联脚本** 三处必须一致，改一处要同步三处。
- **accent 上的文字色**用 `theme.ts` 导出的 `accentTextOnAccent(accent)`（按 WCAG 相对亮度算黑 / 白），不要写死白字——浅色 accent 下白字看不清。
- 深色主题下**按压态是「变亮」**（`lighten(base, 0.1)`），浅色下才是变暗。这是 Fluent 的 surface 提亮习惯。

### 3.2 圆角：随控件尺寸比例缩放（≈ height × 0.28）

| 档 | 值 | 场景 |
|---|---|---|
| tiny | 6px | 迷你控件 |
| small | 8px | `size="small"` 控件（含 `naive` 的 `.n-input--small` 回补） |
| medium | 10px | 默认尺寸控件 / `.field` |
| large | 12px | 大控件 |
| container | 12px | 卡片 / 弹窗 / `.fluent-card` / 保存条 |
| 胶囊 | 999px | `.badge` / `.pill` / `.topbar-status` / `.switch`（**不参与比例体系，唯一例外**） |

统一写入 `theme.ts` 的 `Button` / `Card` / `Dialog` / `Input` / `Tag` 覆盖，以及 `global.css` 的 `.field` / `.switch`。**不要在页面 scoped 里再定义一份。**

> `naive-ui` 把 `--n-*` 写在组件根元素的 inline style 上，覆盖它需要 `!important`（见 `.n-input--small` 的写法）。

### 3.3 动效时长

```
--dur-standard    0.2s   ── 基准（与 --fluent-transition 联动）
--dur-standard-lg 0.25s  ── 略重（列表 / 网格 FLIP）
--dur-settle      0.28s  ── 元素归位 / toast 离场
--dur-toast       0.3s   ── toast 入场 / 位移
--dur-page-leave  0.167s ── 页面离开（≈ enter × ⅔）
--dur-reveal      0.4s   ── 入场显现
--ease-standard   cubic-bezier(0.33, 0, 0, 1)
--delay-step      60ms   ── 错峰步进
```

规则：

- **enter / leave 不对称**：leave ≈ enter × ⅔（`--dur-page-leave` 就是为此存在）。
- **微交互（hover / active / 小位移）固定 `0.15s`**，不吃倍率 token。
- **滑动指示条（如侧栏 `nav-indicator`）用回弹 easing `cubic-bezier(0.2, 0.9, 0.25, 1)`**（时长配 `--dur-settle`），不要用标准 easing——回弹让「跟随选中项移动」有物理感。
- 时长统一 `var(--dur-*)` 驱动，**禁止硬编码 `0.2s` 之类**。

### 3.4 入场秩序（错峰）

- 页面级卡片：`class="enter-stagger"` + `:style="{ animationDelay: staggerMs(n) + 'ms' }"`，`n` 是区块序号（60ms × n）。
- 列表条目：用 `cachedStaggerMs(id)`（按**稳定 id** 缓存 delay），防止列表换序 / 重渲染时重播动画闪烁。
- **条件渲染的区块会让序号前移**：例如 DeepSeek 下隐藏了两张 ZCode 卡，后面的 `staggerMs` 序号要相应前移，否则入场动画出现空档延迟（见 `ImportPage.vue` 的 `showZcodeFormat` 处理）。

### 3.5 数字与文本

- 计数 / 大小 / 时间 / 延迟：`class="tnum"`（`font-variant-numeric: tabular-nums`）。
- 路径 / ID / 日志：`class="mono selectable"`（等宽 + 豁免全局禁选）。
- 长短会变的 chip 用 `min-width: Nch` 防抖（如顶栏路径胶囊 `14ch` 下限）。
- chip / tag 底色用 `color-mix(in srgb, var(--accent) 8~14%, transparent)` 分档，文字用 `--accent-text-strong`。
- 时间戳展示统一走 `formatTime` 类做法（不要直出 `T…Z` 原始串）。

### 3.6 兼容性回退（WebView2 版本差异）

- **`interpolate-size: allow-keywords` 需要 WebView2 129+**，用于原生 `height: 0 ↔ auto` 过渡（展开/收起折叠区、toast 离场高度折叠）。低版本必须带 **`@supports not (interpolate-size: allow-keywords)`** 回退（折叠区退化为 `grid-template-rows: 0fr → 1fr`，toast 退化为脱流绝对定位淡出 + FLIP 补位）。
- 交付底线：低版本**退化为无动画**即可，**绝不可白屏或内容不可见**。
- `color-mix()` 与 WCAG 亮度计算已在 WebView2 验证过，优先沿用现有写法，不另造轮子。

---

## 4. 复用层（优先用这些，别重造）

### 4.1 组件 `components/ui/`

| 组件 | 用途 | 约定 |
|---|---|---|
| `PageHeader.vue` | 统一页头（标题 + 副标题 + 可选图标 + 右侧 actions 插槽） | **Import / Keychain / Migrate / Options / RestorePoint 五页都用**；垂直居中而非 `flex-end`（带副标题时按钮不会偏低）。`ManagePage` 是侧栏 + 编辑区布局，无页头 |
| `StickySaveBar.vue` | 脏态底部浮出「未保存 · 放弃 · 保存」胶囊 | 用于编辑区表单页 |
| `UiDialogHost.vue` | 主题化 confirm / prompt 宿主 | 挂在 `App.vue`，页面不要自己再放一个 |

### 4.2 composable `composables/`

| composable | 用途 | 关键点 |
|---|---|---|
| `useToast.ts` | toast 队列 | hover 暂停按 `createdAt` 结算剩余时长；最多 5 条挤掉最旧；`window.__toast(type,title,msg)` 兼容桥保留 |
| `useStagger.ts` | 错峰 delay | `staggerMs(index)` 页面级；`cachedStaggerMs(id)` 列表级（稳定 id 缓存，>240 自动清） |
| `useHighlight.ts` | 搜索命中分段 | 返回 `Seg[]` 供模板 `<mark v-if="s.hit">` 渲染；**禁 `v-html`**（大小写不敏感，转义后分段） |
| `useVendorGroup.ts` | 厂商分组/筛选（管理页 + 导入页唯一实现） | `matches` 串进页面自己的 `filteredCards`（筛选属可见性、参与批量作用域）；`groupCards(filtered)` 出分组结构；`fetchVendorMeta(id)` 给手动添加的卡片补 `_vendor` / `_supported_levels` |
| `useCapabilityFill.ts` | 能力位批量补全（管理页 + 导入页唯一实现） | 选项由**当前模型集**里「可补全」的位动态生成并带计数（位 ID 命名空间 `in:` / `out:` / `flag:`）；`fill()` **只补空缺**（已有值一律不覆盖）；空选 = 补全部可补项；列表换掉后自动收敛失效选择；汇报走 `useToast` |

### 4.3 规范控件类（`global.css`）

`.field`（输入框）/ `.switch`（自绘开关，`:active` 缩放）/ `.check`（自绘勾选）/ `.badge` + `.badge-ok|warn|err` / `.pill`（档位胶囊）/ `.pill.inferred`（能力位「推断支持、尚未采纳」的弱化变体）/ `mark`（命中高亮）/ `.vendor-group` + `.vendor-group-head|name|count`、`.vendor-group-toggle`（厂商分组视图，两页共用）。

- 页面里出现同名重复定义时，**删掉改用全局的**（这是 C3/B10 的收口目标）。
- 自绘交互件必须补 a11y：`role` / `aria-checked` / `tabindex=0` + Enter/Space（`.pill`、`.switch` 已就位，照抄）。
- 表格补 `th scope="col"`；状态播报行补 `aria-live="polite"`。

---

## 5. 组件选型：naive 还是自绘

| 场景 | 选择 |
|---|---|
| 表单控件（checkbox / radio / switch / input / select） | **naive-ui**（已接 `themeOverrides`） |
| 按钮 | **naive-ui** `n-button`；主操作 `primary` + `#icon`；行内图标操作 `quaternary circle size="small"` 且包 `n-tooltip`；危险 `error` + 必经确认层 |
| 行内批量栏 / 档位选择 | 自绘 `.pill`（token 化） |
| 徽标 / 状态 | 自绘 `.badge`（语义色三档） |

**禁止同屏两套**（D4）：不要一个页面里既用 `n-checkbox` 又有自绘 checkbox 干同一件事。二选一后统一。

---

## 6. 行为正确性（D10 教训）

UI 批次顺手修掉的一类「假成功」问题，属于必须守住的底线：

- **失败不得伪装成功**：后端 setter 返回 Promise 结果，失败要**回滚本地乐观更新** + 弹错误 toast（见 `stores/setting.ts` 的 `trackSetter`）。不要先弹成功 toast 再吞掉错误。
- **失败 ≠ 空态**：数据加载失败要显示 **loading 态 + 错误态**，不能伪装成「暂无条目」。
- **异步期间要有占位**：fetch 期间旧列表不能无反馈，加 loading 遮罩 / spinner。
- **只读状态要语义正确**：按钮文案与全局状态一致（如「显示全部」的反语义要修）。
- **能力位是三态，不要编造「不支持」**：勾选 = 写入 `true`，**取消勾选 = 把该键删掉（= 未知）**，UI 绝不写 `false`；能力数据也不要在渲染期自动写入（逐个「采纳推断」必须由用户点击）。文案不得承诺「取消即让客户端不再支持」——后端只写推断为真的位，未勾选的含义是「交给目标客户端自己的默认值」。

Go 侧 `Set*` 不抛错，业务失败走 `resolve` 的 `{success:false,error}` 载荷——前端要**同时**把 binding reject 与 `success===false` 归一化成 rejection。

---

## 7. i18n 约定

- 三语字典：`zh` / `en` / `ja`，**必须同步齐全**（`i18n/index.ts`）。
- 字典只存文本，**不带 emoji 图标前缀**。
- 新页面 / 新控件文案必须入典，不硬编码中文（历史遗留的硬编码要随改动补上，如本轮补齐了 `KeychainPage` / `MigratePage` / `ImportPage` 模板层）。
- 不顺手改文案措辞（术语统一另议），只做「入典 + 去 emoji」。
- 占位符语义保留（如 `{n}`、`{error}`、`{lang}`）。

---

## 8. 主题与冷启动

- 主题值 `light` / `dark` / `system`（`system` 按 `prefers-color-scheme` 解析）。
- **冷启动防白闪**：`index.html` `<head>` 的**同步内联脚本**在 CSS 包加载前读 `localStorage` 主题并把 `<html>` 底色涂对；非法值回退 `prefers-color-scheme`。改了主题键名或底色要同步这个脚本。
- 本地键名统一 `apm:*` 前缀（历史 `zcode-pm:*` 保留读取以兼容迁移）。

---

## 9. 无障碍基线（D9）

- 全局 `:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px }`；naive 自带聚焦态的控件抑制双环。
- 自绘控件补 `role` / `aria-*` / 键盘可达。
- toast 容器 `aria-live="polite"`；状态播报行同样。
- body 全局禁选，只给 `input` / `textarea` / `.selectable` 豁免。

---

## 10. 自查清单（提交前）

- [ ] emoji 扫描结果为 0（源码 + i18n 字典）。
- [ ] 无 `prompt()` / `confirm()` / `alert()`。
- [ ] 无写死的语义色 / 圆角 / 动画时长，全部走 token。
- [ ] 新页面用了 `PageHeader`，入场接了 `enter-stagger`（序号无空档）。
- [ ] 新文案已入 zh/en/ja 三语。
- [ ] 自绘控件有 a11y 属性。
- [ ] 用到 `interpolate-size` 的展开 / 折叠都带 `@supports not` 回退（低版本退化为无动画，不白屏）。
- [ ] dark / light 双主题各过一遍；tab 键可遍历交互件。
- [ ] 失败路径有错误反馈，不伪装成功 / 不伪装空态。
- [ ] 未触碰 Go 侧业务逻辑（见第 11 节红线）。
- [ ] `cd frontend && npm run build`（含 `vue-tsc`）通过。

---

## 11. 改动范围红线

UI 整改与后续前端改动**只动前端层**，不得顺手动业务逻辑：

- 不碰配置读写、保存 / 恢复 / 迁移 / DPAPI / 凭据解析等 Go 侧业务路径；前端行为修复只限错误处理、loading 态与展示层。
- 不新增 UI 库（见第 2 节红线 3）、不推翻「侧栏 + 顶栏 + 内容区」骨架与路由组织。
- 不引入参考项目（`reference/VrcFrameLimit`）的 Dashboard / 图表 / 性能模式等与 APM 场景无关的模块。
- 遵守 [`AGENTS.md`](AGENTS.md)：不读写真实 `%USERPROFILE%` 配置、`.dsh` 凭据、API Key、备份；测试用占位值。

---

## 12. 验证

```text
cd frontend && npm ci && npm run build   # build 内含 vue-tsc --noEmit
```

前端无 E2E 基建；涉及 WebView2 / DPAPI / 剪贴板 / 主题闪烁的行为改动，需在真实 Wails 窗口目测。完整发布验证见 [`AGENTS.md`](AGENTS.md)。
