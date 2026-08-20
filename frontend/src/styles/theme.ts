import { darkTheme, type GlobalTheme, type GlobalThemeOverrides } from 'naive-ui'

function clamp(n: number): number { return Math.min(255, Math.max(0, n)) }
function hexToRgb(hex: string): [number, number, number] {
  let h = hex.trim().replace(/^#/, '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  const num = parseInt(h, 16)
  if (Number.isNaN(num)) return [0, 103, 192]
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

export function buildThemeOverrides(accent: string, dark?: boolean): GlobalThemeOverrides {
  const base = hexToRgb(accent)
  const hover = lighten(base, 0.12)
  const pressed = dark ? lighten(base, 0.1) : darken(base, 0.12)
  const paletteBase = rgbToHex(base[0], base[1], base[2])
  const paletteHover = rgbToHex(hover[0], hover[1], hover[2])
  const palettePressed = rgbToHex(pressed[0], pressed[1], pressed[2])
  return {
    common: {
      primaryColor: paletteBase,
      primaryColorHover: paletteHover,
      primaryColorPressed: palettePressed,
      primaryColorSuppl: paletteBase,
      borderRadius: '8px',
      borderRadiusSmall: '6px',
      fontFamily: "'Segoe UI Variable Display','Segoe UI Variable','Segoe UI',system-ui,-apple-system,sans-serif",
      fontSize: '14px',
      ...(dark ? { bodyColor: '#202020', cardColor: '#2b2b2b', modalColor: '#2b2b2b' } : {}),
    },
    Button: { borderRadiusMedium: '6px' },
    Card: { borderRadius: '10px' },
    Dialog: { borderRadius: '10px' },
    Menu: { itemHeight: '40px', borderRadius: '8px' },
    Switch: { railColorActive: accent },
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
