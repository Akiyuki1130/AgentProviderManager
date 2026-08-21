export type UiLang = 'zh' | 'en' | 'ja'

const dict: Record<string, Record<UiLang, string>> = {
  'subtitle': {
    zh: '管理多 Agent 的第三方模型提供商',
    en: 'Manages providers for multiple agents',
    ja: '複数のエージェント向けプロバイダーを管理',
  },
  'tab.manage': { zh: '管理配置', en: 'Manage', ja: '設定管理' },
  'tab.import': { zh: '导入提供商', en: 'Import Provider', ja: 'プロバイダーをインポート' },
  'tab.migrate': { zh: '配置文件迁移', en: 'Migrate Config', ja: '設定ファイル移行' },
  'tab.keychain': { zh: 'API Key 钥匙串', en: 'API Key Keychain', ja: 'APIキー管理' },
  'tab.options': { zh: '选项', en: 'Options', ja: 'オプション' },
  'topbar.chooseCfg': { zh: '选择配置文件...', en: 'Choose config...', ja: '設定ファイルを選択...' },
  'topbar.openDir': { zh: '打开配置目录', en: 'Open config dir', ja: '設定フォルダを開く' },
  'topbar.restore': { zh: '恢复最近备份', en: 'Restore recent backup', ja: '最近のバックアップを復元' },
  'topbar.noBackup': { zh: '无备份可恢复', en: 'No backup to restore', ja: '復元するバックアップなし' },
  'options.title': { zh: '选项', en: 'Options', ja: 'オプション' },
  'options.lang': { zh: '语言', en: 'Language', ja: '言語' },
  'options.langTag': { zh: 'language', en: 'language', ja: 'language' },
  'options.langDesc': {
    zh: '界面显示语言，默认跟随系统',
    en: 'UI language, follows the system by default',
    ja: '画面表示言語。既定ではシステムに従います',
  },
  'options.langSystem': { zh: '跟随系统', en: 'System', ja: 'システム' },
  'options.langSystemHint': {
    zh: '（当前为{lang}）',
    en: ' (currently {lang})',
    ja: '（現在は{lang}）',
  },
  'options.zh': { zh: '中文', en: 'Chinese', ja: '中国語' },
  'options.en': { zh: 'English', en: 'English', ja: '英語' },
  'options.ja': { zh: '日本語', en: 'Japanese', ja: '日本語' },
  'options.theme': { zh: '外观主题', en: 'Appearance', ja: '外観テーマ' },
  'options.themeDesc': {
    zh: '白色 / 黑色 / 跟随系统',
    en: 'Light / Dark / Follow system',
    ja: 'ライト / ダーク / システムに従う',
  },
  'options.themeLight': { zh: '白色', en: 'Light', ja: 'ライト' },
  'options.themeDark': { zh: '黑色', en: 'Dark', ja: 'ダーク' },
  'options.accent': { zh: '主题颜色', en: 'Accent color', ja: 'アクセントカラー' },
  'options.accentDesc': {
    zh: '按钮、高亮等强调色，可自选颜色',
    en: 'Accent color for buttons and highlights',
    ja: 'ボタンや強調表示のアクセントカラー',
  },
  'options.http': { zh: 'HTTP 支持', en: 'HTTP support', ja: 'HTTP サポート' },
  'options.httpDesc': {
    zh: '默认关闭。开启后允许使用 http:// 协议；https:// 始终受支持',
    en: 'Off by default. When enabled, http:// URLs are allowed; https:// is always supported',
    ja: '既定ではオフ。有効にすると http:// を許可します。https:// は常に利用可能',
  },
  'options.autofill': {
    zh: '导入时自动补全上下文与输出长度',
    en: 'Auto-fill context & output length on import',
    ja: 'インポート時にコンテキストと出力長を自動入力',
  },
  'options.autofillDesc': {
    zh: '获取模型时若未返回上下文/输出长度，则按常见模型预设自动填写',
    en: 'When fetching models, fill missing context/output from common-model presets',
    ja: 'モデル取得時にコンテキスト/出力長が無ければ、一般的なモデル設定から自動入力します',
  },
  'options.intensity': {
    zh: '模型思考默认全部强度',
    en: 'All intensities when toggling reasoning',
    ja: '推論ONで全強度を選択',
  },
  'options.intensityDesc': {
    zh: '打开模型（含批量）的「思考」开关时默认勾选全部推理档位；关闭则只勾选 off/high/max',
    en: 'When toggling reasoning on (single or batch), enable all intensity levels; off selects only off/high/max',
    ja: 'モデル（バッチ含む）の「推論」をONにした際に全強度を選択します。オフなら off/high/max のみ',
  },
  'btn.fetch': { zh: '🔄 获取模型列表', en: '🔄 Fetch models', ja: '🔄 モデルを取得' },
  'btn.clear': { zh: '清空', en: 'Clear', ja: 'クリア' },
  'btn.import': { zh: '⬇ 导入到当前 Agent', en: '⬇ Import to current agent', ja: '⬇ 現在のエージェントにインポート' },
  'btn.saveProvider': { zh: '💾 保存提供商', en: '💾 Save provider', ja: '💾 プロバイダーを保存' },
  'btn.deleteProvider': { zh: '🗑 删除提供商', en: '🗑 Delete provider', ja: '🗑 プロバイダーを削除' },
  'btn.duplicateProvider': { zh: '📋 复制提供商', en: '📋 Duplicate', ja: '📋 複製' },
  'btn.refreshProvider': { zh: '🔄 重新获取模型', en: '🔄 Refresh models', ja: '🔄 モデルを再取得' },
  'btn.addModel': { zh: '+ 手动添加模型', en: '+ Add model manually', ja: '+ モデルを手動追加' },
  'btn.addProvider': { zh: '+ 新增提供商', en: '+ New provider', ja: '+ プロバイダーを追加' },
  'status.ready': {
    zh: '就绪。请输入地址和密钥后点击获取。',
    en: 'Ready. Enter Base URL and API key, then click Fetch.',
    ja: '準備完了。Base URL と API キーを入力して取得をクリックしてください。',
  },
  'status.requesting': { zh: '正在请求 /models 接口...', en: 'Requesting /models...', ja: '/models をリクエスト中...' },
  'common.ok': { zh: '确定', en: 'OK', ja: 'OK' },
  'common.cancel': { zh: '取消', en: 'Cancel', ja: 'キャンセル' },
}

export function t(key: string, lang: UiLang): string {
  const entry = dict[key]
  if (!entry) return key
  return entry[lang] || entry['zh'] || key
}
