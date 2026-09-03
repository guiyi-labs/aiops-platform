import { ref, watchEffect } from 'vue'

type Theme = 'dark' | 'light'

const THEME_KEY = 'aiops.theme'

function getInitialTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_KEY)
    if (stored === 'dark' || stored === 'light') return stored
  } catch {
    // localStorage may be unavailable (private mode / disabled cookies)
  }
  return 'dark' // default to dark (current look)
}

const theme = ref<Theme>(getInitialTheme())

watchEffect(() => {
  const root = document.documentElement
  root.setAttribute('data-theme', theme.value)
  if (theme.value === 'light') {
    root.classList.add('theme-light')
    root.classList.remove('theme-dark')
  } else {
    root.classList.add('theme-dark')
    root.classList.remove('theme-light')
  }
  try {
    localStorage.setItem(THEME_KEY, theme.value)
  } catch {
    // Persistence is best-effort; theme still applies for this session
  }
})

export function useTheme() {
  function toggle() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
  }

  function set(t: Theme) {
    theme.value = t
  }

  return {
    theme,
    toggle,
    set,
    isDark: () => theme.value === 'dark',
    isLight: () => theme.value === 'light',
  }
}
