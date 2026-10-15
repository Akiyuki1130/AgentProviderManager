# AgentProviderManager UI/UX 优化计划（已归档）

> **状态（2026-10-14）**：批次 1–6 全部完成并通过全量验证，随后又有两轮修复（顶栏拖拽 / 越大越圆的半径比例 / 尺寸对齐清扫 / accent 色板拾色器 + toast 级联离场 / 侧栏字重 / 导入隔离；顶栏图标收缩 / 路径中段省略 / ZCode 格式卡按 Agent 门控）。**全部改动已提交**：`bac34f6`（六批）+ `0efe678`（修复轮），工作区干净。
>
> **本文件已归档为历史记录**（现存于 `Archive/`）：长期生效的前端约定已抽取到 [`UI-GUIDELINES.md`](../UI-GUIDELINES.md)，后续前端改动以该文件为准；本文只保留当时的诊断与逐条差距清单供追溯。
>
> ⚠️ 本文正文（尤其第 3 节「规格基准」）保留的是**计划落盘时**的数值，部分已被后续修复轮修正，**不要照抄**。已知过期项：① 圆角——原定「控件 6 / 块 8 / 卡片 10」，实际改为**随控件尺寸比例缩放**（≈height×0.28：tiny 6 / small 8 / medium 10 / large 12 / 容器 12，胶囊 999px 例外）；② 弹窗宽度两档 420/480 仍有效。以 [`UI-GUIDELINES.md`](../UI-GUIDELINES.md) 第 3 节为准。
>
> B12 命令面板按约定不做，留 [`TODO.md`](../TODO.md)。
>
> 参考基线：`reference/VrcFrameLimit`（本地克隆，仅作设计参考，已加入 `.gitignore`，不入库）。
> 计划落盘时的代码基线：`master @ 0861193`。文中 `file:line` 引用以该基线为准。

---

## 0. 决策记录（已与用户确认）

| 决策点 | 结论 |
|---|---|
| 图标 | **1000000% 禁止 emoji**。统一使用项目已有依赖 `@vicons/ionicons5`（MIT 开源，与参考项目同款），经 naive-ui `<n-icon>` + 按钮 `#icon` 插槽渲染 |
| 行为缺口（假成功 toast、失败伪装空态、无 loading 等） | **本次一并修掉**（改动限于前端层，不动 Go 后端） |
| i18n | **清洗文案 emoji + 补齐 KeychainPage / MigratePage 整页接入**，消灭同页双语夹杂 |
| 交付节奏 | **分批交付**：批次 2 结束先给用户过目目测，再继续后续批次 |
| 范围红线 | 不动配置读写业务逻辑、不引入新 UI 库、不推翻「侧栏 + 顶栏」布局骨架、不搬参考项目的 Dashboard/图表/性能模式 |

---

## 1. 现状诊断

APM 已移植 VrcFrameLimit 的 Fluent 骨架（CSS 变量双主题、accent、侧栏滑动指示条、fade-slide 路由转场、fluent-card），但精致度差距是系统性的，病根四层：

1. **Emoji 当图标** —— 全项目 40+ 处（按钮、弹窗标题、徽标、状态字符 `✔✕⚠`、i18n 三语字典）。多色位图字形：不跟 accent、基线不齐、dark 刺眼、无法控尺寸。参考项目全项目零 emoji。
2. **原生 OS 弹窗** —— `prompt()/confirm()` 共 8 处，主题/圆角/字体/动效全断裂。参考项目对应场景是主题化 `n-modal` 表单 + `dialog.warning`。
3. **自制控件缺精致基因 + dark 硬伤** —— `.switch` 轨道 `rgba(0,0,0,.18)` 深底隐形、`.check-mark` 边框 `#888`、语义色无体系。
4. **缺少秩序感** —— 无页头、无错峰入场、状态切换硬切、时长/圆角/尺寸各页随意。

---

## 2. 差距清单（A–D 全量）

### A · 借鉴参考补齐（动效与交互件）

- A1 动效 token 补全：`--dur-settle .28s / --dur-toast .3s / --dur-page-leave .167s / --dur-reveal .4s / --delay-step 60ms`，替换全站硬编码时长。
- A2 Toast 升级：语义图标（Checkmark/Alert/Close/InformationCircle）、关闭按钮、**hover 暂停倒计时**（按 createdAt 结算 remaining）、最多堆叠 5 条挤掉最旧、`aria-live="polite"`、`column-reverse` 新条在下往上冒。对外 `window.__toast(type, title, msg)` 接口保持不变。
- A3 StickySaveBar：编辑区脏时底部浮出「未保存的更改 · 放弃 · 保存」胶囊；复用 `ManagePage.isDirty` 与 `leaveGuard`（`stores/leaveGuard.ts` 已具备 registerManageGuard(save/discard) 三件套）。
- A4 FLIP 列表过渡：全局 `.list-*`（leave 元素 `position:absolute` 保持原位、其余 FLIP 补位），挂到 ManagePage 提供商列表与模型列表、Keychain 列表、导入结果列表。
- A5 ModelCard 展开/收起：`v-if` 硬弹改为高度过渡（主路径 `interpolate-size: allow-keywords`，WebView2 129+；回退 `grid-template-rows 0fr→1fr`，参考 `SettingsPage.vue:290-323` 的 fi-collapse 对）。

### B · 更进一步（APM 场景特有）

- B6 编辑状态可视化：侧栏 provider 项脏标记橙点；顶栏目标路径改状态胶囊（accent 14% 底 + `min-width` 防抖）。
- B7 键盘效率：`Ctrl+S` 保存当前提供商；`Esc` 清空搜索框；保存按钮 title 提示快捷键。
- B8 测试结果反馈：probe 结果条滑入动画；延迟数字 `tabular-nums`；图标 + 色彩 token 分段（替代字符串里的 `✔✕⚠`）。
- B9 搜索命中高亮：提供商/模型匹配子串 `<mark>` 高亮（转义后分段渲染，禁 v-html）。
- B10 控件样式去重：`.switch/.badge/.field/pill` 收编入 global.css 单一出处。
- B11 空状态可操作化：图标 + 可点击引导按钮（空状态直接触发「新增提供商」）。
- ~~B12 Ctrl+K 命令面板~~：用户未点名，**不做**，留 TODO.md。

### C · 控件精致化（病根对症）

- C1 图标化（本计划核心）：
  - 按钮：`<template #icon><n-icon :component="XxxOutline"/></template>` 模式。建议映射：保存=SaveOutline / 删除=TrashOutline / 获取刷新=SyncOutline / 测试连接=FlashOutline / 钥匙串=KeyOutline / 存入钥匙串=DownloadOutline / 从钥匙串=KioskOutline 或 KeyOutline / 导入=CloudDownloadOutline / 导出=CloudUploadOutline / 添加(+)=AddOutline / 复制=CopyOutline / 移动箭头=Chevron。
  - 状态字符 `✔✕⚠✓✗` → 图标 + 语义色 class（`ImportPage.vue:216-233`、`ManagePage.vue:548-551`、toast 正文里的 ✓、徽标里的 ✓/🔗）。
  - `ModelCard` 的 `✕`/chevron SVG → `CloseOutline`/`ChevronDown`（保留现有 chevron 可换 n-icon 统一）。
  - `MigratePage.vue:11` 的 `→` 字符箭头 → `ArrowForwardOutline`。
  - i18n 字典清洗：`i18n/index.ts:105-111,281-282` 等 9 条文案里的 emoji 前缀移除，图标在模板层加，不进文案。
- C2 弹窗化（8 处原生对话框，逐一替换且保留原语义与守卫）：
  - `ManagePage.vue:338 prompt(新增提供商名)`、`:406 prompt(添加模型ID)`、`ImportPage.vue:269 prompt(添加模型ID)` → 主题 `n-modal` 小表单（label + n-input + 校验 + Enter 提交）。
  - `ManagePage.vue:456,485 / ImportPage.vue:278,338 / MigratePage.vue:208 confirm(...)` → `dialog.warning` 或就近 `n-popconfirm`（与既有 KeychainPage/RestorePointPage 的 popconfirm 风格统一：删除类=popconfirm，跨行影响类=dialog）。
- C3 控件规范层：switch/check/badge/field/pill 全局单一定义；switch 修 dark（轨道 `color-mix(in srgb, var(--fluent-text) 22%, transparent)` 类做法）；全部交互件补 `:active` 与过渡。
- C4 按钮语义分级：主操作=primary+icon；行内图标操作=`quaternary circle size="small"`（参考 ModeCard 按钮组，每个包 `n-tooltip`）；危险=error ghost + 必经确认层；废除内联 `color="#107c10"/#ca5010"`（改语义 class，仍跟主题）。
- C5 统一页头 + 错峰入场：抽 `PageHeader`（h1 + 副标题 + 右侧主操作区）；入场 `fade-in var(--dur-reveal) both` + `animation-delay = 60ms×n`（页面卡），列表卡 `60 + 40×index` 且**按稳定 id 缓存 delay**（防换序重播闪烁，参考 `useDragReorder.ts:55-63`）。
- C6 数字与状态细节：计数/大小/时间/延迟一律 `font-variant-numeric: tabular-nums`；chip `min-width: Nch` 防长短抖动；ID 类文本 `Consolas,'Cascadia Code',monospace` 统一。

### D · 深挖新增差距

- D1 语义色体系：global.css 新增 `--ok/--warn/--err`（light/dark 各一套，dark 提亮降饱和，参考 theme.ts warning 双色板思路）；替换五处硬编码 `#107c10/#d13438/#ca5010` + OptionsPage 的第五种红 `#c42b1c`。
- D2 dark 硬伤：switch 轨道/check-mark 边框/白滑块阴影/badge 8% tint（`ImportPage.vue:515`、`MigratePage.vue:264` 等，随 C3 一并修）。
- D3 确认交互统一：三套并存 → 按 C2 收敛为两套（popconfirm + dialog/modal）。
- D4 同类控件二选一：表单类统一 naive（n-checkbox/n-radio/n-switch，themeOverrides 已有），行内批量栏保留自绘 pill 但 token 化；杜绝同屏两套。
- D5 规格统一：
  - 圆角三级节奏：控件 **6px** / 常规块 **8px** / 卡片 **10px**（与 theme.ts Button 6 / common 8 / Card 10 对齐）；badge 999px 唯一例外。
  - theme.ts 补齐组件覆盖：`Input{borderRadius:6}`、`Tag{borderRadius:5}`、`Menu{itemColorActive:accent 10%、暗色 activeText lighten}`、Button 三态文字锁 `textOnAccent`、**dark 下 pressed=lighten(base,0.1)**。
  - modal 宽度两档制：简单确认 420 / 表单 480；工具条按钮统一 `size="small"`。
  - `--fluent-radius` 死 token：或接线使用或删除（倾向删除，语义化 6/8/10 常量替代）。
- D6 数字排版：见 C6；另统一时间戳展示（KeychainPage 直出 `T…Z` → 复用 `formatTime` 类做法，向 RestorePointPage 看齐）。
- D7 状态切换补齐：`EditContextMenu` 先 clamp 坐标再渲染（修右缘跳帧）+ 淡入缩放过渡；compat 卡/密码面板/更新提示行接 fi-collapse 或 fade-slide；所有 v-for 列表接 A4 FLIP。
- D8 冷启动白闪：`frontend/index.html` `<head>` 加同步内联脚本——读 localStorage 主题（非法值回退 `prefers-color-scheme`），在 CSS 包前把 `<html>` 底色涂对（`#202020/#f3f3f3`，与 global.css、theme.ts 三处颜色一致）。滚动条隐藏（`::-webkit-scrollbar{width:0}`）；body 禁选保留，新增 `.selectable` 豁免给路径/ID/日志类文本。
- D9 a11y 基线：全局 `:focus-visible { outline:2px solid var(--accent); outline-offset:2px }`；自绘 switch 补 `role="switch" aria-checked`；pill 补 `role="button" tabindex=0` + Enter/Space；状态播报行补 `aria-live="polite"`；表格补 `th scope="col"`。
- D10 行为缺口修复（已批准一并修）：
  - `OptionsPage.vue:189-204`：开关先弹成功 toast，而 `stores/setting.ts:102-116` fire-and-forget 吞错 → setter 返回 Promise 结果，失败弹错误 toast。
  - `KeychainPage.vue:111-119`、`KeychainPicker.vue:53-58`：加载失败伪装「暂无条目」→ 增加 loading 态 + 错误态（错误 ≠ 空态）。
  - `ImportPage.vue`：fetch 期间旧列表无占位 → 列表区 loading 遮罩/spinner。
  - 批量上下文/输出长度：仅回车可应用无提示 → placeholder 改「输入后回车应用」并加小应用按钮。
  - `KeychainPage.vue:140 isRevealed` showAll 反语义 → 按钮文案与全局状态一致。
- D11 i18n 补齐（已批准）：`KeychainPage.vue`、`MigratePage.vue` 整页接入 `t()`（zh/en/ja 三语）；`ImportPage.vue` 模板层硬编码中文全量入典；`main.ts:15` 全局错误 toast 入典；清理字典死 key（`btn.fetch` 等 6 条：能接线的接线，接不上线的删除）。

---

## 3. 规格基准（全批次共用的硬数值）

| 项 | 值 | 来源 |
|---|---|---|
| 底色 light / dark | `#f3f3f3 / #202020`，卡片 `#fff / #2b2b2b` | global.css 已有，index.html 内联需三处一致 |
| 圆角 | 控件 6 / 块 8 / 卡片 10 / 胶囊 999 | theme.ts + ModeCard |
| easing | 标准 `cubic-bezier(.33,0,0,1)`；指示条回弹 `cubic-bezier(.2,.9,.25,1)`；微交互固定 0.15s 不吃倍率 | global.css / AppSidebar |
| 时长 token | standard .2 / standard-lg .25 / settle .28 / toast .3 / page-leave .167 / reveal .4 | global.css |
| 入场 stagger | 页面卡 60ms×n；列表卡 60+40×index（id 级缓存） | SettingsPage / useDragReorder |
| enter/leave 不对称 | leave ≈ enter × 2/3 | App.vue 路由、向导 |
| accent 文字 | 底色上：WCAG luminance >0.5 ? 黑 : 白；文本上：dark 用 `color-mix(accent 45%, white)` | theme.ts:71-95 / global.css |
| focus-visible | `outline 2px solid accent, offset 2px`（滑轨 4px） | QuickPresets 等 |
| chip/tag | `color-mix(accent 8~14%, transparent)` 分档 + 文字 `--accent-text-strong`；`min-width:Nch` 防抖 | ModeCard / AppTopbar |
| toast | 左下 `column-reverse`、max 5、hover 暂停、enter 滑入/leave 原位淡出 | useToast / ToastContainer |
| 图标 | 仅 `@vicons/ionicons5`；按钮内 16–18px；零 emoji 零 unicode 状态字符 | 参考项目全案 |

---

## 4. 分批执行计划

**每批完成后必须**：`cd frontend && npm run build` 通过 + 该批涉及交互自查；批次 2 结束后交用户目测一轮再继续。

### 批次 1 · 地基
- global.css：D1 语义色 token、A1 时长 token、A4 `.list-*`、fade-in keyframes、D8 滚动条隐藏 + `.selectable`、D9 focus-visible 全局规则、C6 tabular-nums 工具类。
- theme.ts：D5 组件覆盖补齐 + `accentTextOnAccent` 导出 + dark pressed 逻辑。
- index.html：D8 内联主题脚本。
- `.gitignore`：追加 `reference/`。
- DoD：构建过、现有页面外观零回归（只加 token 未接线）。

### 批次 2 · 图标化 + 弹窗化（⏸ 完成后给用户过目）
- C1 全量 emoji/状态字符 → ionicons5（含 i18n 文案清洗）。
- C2 八处 prompt/confirm → 主题 modal 表单 / popconfirm / dialog。
- C4 按钮语义分级 + 去内联色（D1 语义 class 接线）。
- DoD：目测全页面按钮无 emoji；无任何原生 OS 弹窗可从 UI 触发。

### 批次 3 · 交互件
- A2 Toast 升级（hover 暂停/图标/关闭/上限）。
- A3 StickySaveBar 接入 ManagePage（及 OptionsPage 类表单视情况）。
- A4 FLIP 接线（提供商列表、模型列表、钥匙串列表）。
- A5 ModelCard fi-collapse 展开过渡。
- D7 EditContextMenu 跳帧修复 + 出现动画；其余 v-if 硬切接过渡。
- DoD：删卡补位顺滑、toast 可悬停可关、脏态浮出保存条。

### 批次 4 · 规格统一 + 入场秩序 + a11y
- D4 控件二选一收敛；D5 尺寸/圆角/modal 宽度统一；B10 样式去重落地。
- C5 PageHeader 组件 + 六页接入 + stagger 入场。
- C6 数字排版 + 时间戳统一（D6）。
- D9 全部自绘控件 role/aria/键盘、状态行 aria-live。
- B6/B7/B8/B9/B11 体验增强（脏点、Ctrl+S、probe 反馈、搜索高亮、空状态）。
- D2 残余 dark 硬伤清扫。
- DoD：dark/light 双主题逐页过一遍；tab 键可遍历全部交互。

### 批次 5 · 行为缺口 + i18n 补齐
- D10 五处行为修复（前端层）。
- D11 KeychainPage / MigratePage 整页 i18n + ImportPage 模板入典 + main.ts + 死 key 清理。
- DoD：zh/en/ja 三语逐页无夹杂；失败操作不再出现成功提示。

### 批次 6 · 全量验证
```text
cd frontend && npm ci && npm run build
cd .. && go test ./... && go vet ./... && go mod verify
wails build
```
（构建离线可跑，无需代理；产物核对用记忆中的 exe 新鲜度流程。）

---

## 5. 风险与规避

- **interpolate-size** 需 WebView2 129+：必须带 grid-rows 回退对（参考 SettingsPage 模式），低版本退化为无动画展开，不可白屏。
- **color-mix / WCAG 亮度计算**：沿用参考实现，已在 WebView2 验证过的写法优先。
- **i18n 整页补齐**是大范围文本替换：逐 key 对照，保留占位符语义；不顺手改文案措辞（术语统一不在本次范围）。
- **行为修复**只动前端错误处理与 loading 态，不触碰保存/恢复/DPAPI/凭据路径逻辑；遵守 AGENTS.md（不读写真实 `\.dsh`、密钥、备份）。
- **不 commit**：全程只改工作区，提交与版本号同步由用户另行发起（版本七处同步清单见 AGENTS.md）。
