import { darkTheme, type GlobalTheme, type GlobalThemeOverrides } from 'naive-ui'

export interface AccentPalette {
  base: string
  hover: string
  pressed: string
  subtle: string
  textOnAccent: string
}

const warningLight = { color: '#f0a020', hover: '#fcb040', pressed: '#c97c10', suppl: '#fcb040' }
const warningDark = { color: '#f2c97d', hover: '#f5d599', pressed: '#e6c260', suppl: 'rgb(240, 138, 0)' }

function clamp(n: number): number { return Math.min(255, Math.max(0, n)) }
function hexToRgb(hex: string): [number, number, number] {
  let h = hex.trim().replace(/^#/, '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  const num = parseInt(h, 16)
  if (Number.isNaN(num)) return [0, 120, 212]
  return [(num >> 16) & 255, (num >> 8) & 255, num & 255]
}
function rgbToHex(r: number, g: number, b: number): string {
  const toHex = (n: number) => clamp(Math.round(n)).toString(16).padStart(2, '0')
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`
}
function mix(a: [number, number, number], b: [number, number, number], w: number): [number, number, number] {
  const wt = Math.min(1, Math.max(0, w))
  return [a[0] * (1 - wt) + b[0] * wt, a[1] * (1 - wt) + b[1] * wt, a[2] * (1 - wt) + b[2] * wt]
}
function lighten(rgb: [number, number, number], amount: number): [number, number, number] { return mix(rgb, [255, 255, 255], amount) }
function darken(rgb: [number, number, number], amount: number): [number, number, number] { return mix(rgb, [0, 0, 0], amount) }

function relativeLuminance(rgb: [number, number, number]): number {
  const linear = rgb.map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4)
  })
  return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2]
}

/** 强调色底上的可读文字色：WCAG 相对亮度 >0.5 用黑，否则用白 */
export function accentTextOnAccent(accent: string): string {
  return relativeLuminance(hexToRgb(accent)) > 0.5 ? '#000000' : '#FFFFFF'
}

export function buildPalette(accent: string): AccentPalette {
  const base = hexToRgb(accent)
  const hover = lighten(base, 0.12)
  const pressed = darken(base, 0.12)
  return {
    base: rgbToHex(base[0], base[1], base[2]),
    hover: rgbToHex(hover[0], hover[1], hover[2]),
    pressed: rgbToHex(pressed[0], pressed[1], pressed[2]),
    subtle: `rgba(${clamp(base[0]) | 0}, ${clamp(base[1]) | 0}, ${clamp(base[2]) | 0}, 0.1)`,
    textOnAccent: accentTextOnAccent(accent),
  }
}

export function buildThemeOverrides(accent: string, dark?: boolean): GlobalThemeOverrides {
  const palette = buildPalette(accent)
  const base = hexToRgb(accent)
  // 深色主题下按压态「变亮」（与 surface 提亮习惯一致），浅色下变暗
  const pressed = dark ? lighten(base, 0.1) : darken(base, 0.12)
  const warning = dark ? warningDark : warningLight
  const itemTextColorActive = dark ? rgbToHex(...lighten(base, 0.15)) : palette.base
  return {
    common: {
      primaryColor: palette.base,
      primaryColorHover: palette.hover,
      primaryColorPressed: rgbToHex(pressed[0], pressed[1], pressed[2]),
      primaryColorSuppl: palette.base,
      warningColor: warning.color,
      warningColorHover: warning.hover,
      warningColorPressed: warning.pressed,
      warningColorSuppl: warning.suppl,
      borderRadius: '8px',
      borderRadiusSmall: '6px',
      fontFamily: "'Segoe UI Variable Display','Segoe UI Variable','Segoe UI',system-ui,-apple-system,sans-serif",
      fontSize: '14px',
      ...(dark ? { bodyColor: '#202020', cardColor: '#2b2b2b', modalColor: '#2b2b2b' } : {}),
    },
    // 圆角随控件尺寸缩放（≈height×0.28）：tiny 6 / small 8 / medium 10 / large 12 / 容器 12
    Button: {
      borderRadiusTiny: '6px',
      borderRadiusSmall: '8px',
      borderRadiusMedium: '10px',
      borderRadiusLarge: '12px',
      // 主按钮三态文字锁定 accent 对比色，避免浅色 accent 下白字看不清
      textColorPrimary: palette.textOnAccent,
      textColorHoverPrimary: palette.textOnAccent,
      textColorPressedPrimary: palette.textOnAccent,
    },
    Card: { borderRadius: '12px' },
    Dialog: { borderRadius: '12px' },
    // naive Input 只有单一半径变量，按 medium 档；small 档由 global.css 的 .n-input--small 回补 8px
    Input: { borderRadius: '10px' },
    Tag: { borderRadius: '5px' },
    Menu: {
      itemHeight: '40px',
      borderRadius: '8px',
      itemColorActive: palette.subtle,
      itemTextColorActive,
    },
    Switch: { railColorActive: accent },
    Progress: { fillColor: accent },
  }
}

export function resolveTheme(theme: string, systemDark: boolean): 'light' | 'dark' {
  if (theme === 'system') return systemDark ? 'dark' : 'light'
  return theme === 'dark' ? 'dark' : 'light'
}
export function isDark(theme: string, systemDark: boolean): boolean {
  return resolveTheme(theme, systemDark) === 'dark'
}
export { darkTheme }
export type { GlobalTheme }
