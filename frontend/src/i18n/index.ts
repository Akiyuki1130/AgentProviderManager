export type Lang = 'zh' | 'en'

const dict: Record<string, Record<Lang, string>> = {
  'subtitle': { zh: '管理 ZCode 的第三方模型提供商', en: 'Manages ZCode custom providers' },
  'tab.manage': { zh: '管理配置', en: 'Manage' },
  'tab.import': { zh: '导入提供商', en: 'Import Provider' },
  'tab.keychain': { zh: 'API Key 钥匙串', en: 'API Key Keychain' },
  'btn.fetch': { zh: '🔄 获取模型列表', en: '🔄 Fetch models' },
  'btn.clear': { zh: '清空', en: 'Clear' },
  'btn.import': { zh: '⬇ 导入到 ZCode', en: '⬇ Import to ZCode' },
  'btn.saveProvider': { zh: '💾 保存提供商', en: '💾 Save provider' },
  'btn.deleteProvider': { zh: '🗑 删除提供商', en: '🗑 Delete provider' },
  'btn.duplicateProvider': { zh: '📋 复制提供商', en: '📋 Duplicate' },
  'btn.refreshProvider': { zh: '🔄 重新获取模型', en: '🔄 Refresh models' },
  'btn.addModel': { zh: '+ 手动添加模型', en: '+ Add model manually' },
  'btn.addProvider': { zh: '+ 新增提供商', en: '+ New provider' },
  'status.ready': { zh: '就绪。请输入地址和密钥后点击获取。', en: 'Ready. Enter Base URL and API key, then click Fetch.' },
  'status.requesting': { zh: '正在请求 /models 接口...', en: 'Requesting /models...' },
  'common.ok': { zh: '确定', en: 'OK' },
  'common.cancel': { zh: '取消', en: 'Cancel' },
}

export function t(key: string, lang: Lang): string {
  const entry = dict[key]
  if (!entry) return key
  return entry[lang] || entry['zh'] || key
}
