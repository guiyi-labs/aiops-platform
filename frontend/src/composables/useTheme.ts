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
  // 默认浅色。此前默认写的是 'dark'，而当时 .theme-dark 没有任何规则，
  // 于是「dark」实际渲染成浅色——默认值与外观是对不上的。补了暗色规则之后
  // 这个默认值会真的把界面变暗，所以显式改成 light：保持既有外观不变，
  // 让用户主动选择暗色。
  return 'light'
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
