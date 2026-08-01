const THEME_KEY = 'portalt-theme'

export function useTheme() {
  const isDark = useState<boolean>('portalt-dark', () => false)

  function apply() {
    if (process.client) {
      document.documentElement.classList.toggle('dark', isDark.value)
    }
  }

  function init() {
    if (!process.client) return
    const saved = localStorage.getItem(THEME_KEY)
    if (saved === 'dark' || (!saved && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
      isDark.value = true
    }
    apply()
  }

  function toggle() {
    isDark.value = !isDark.value
    apply()
    if (process.client) {
      localStorage.setItem(THEME_KEY, isDark.value ? 'dark' : 'light')
    }
  }

  return { isDark, init, toggle }
}
