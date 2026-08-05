export function useIsMobile() {
  const isMobile = ref(false)
  let mql: MediaQueryList | null = null

  function update() {
    isMobile.value = !!mql?.matches
  }

  onMounted(() => {
    mql = window.matchMedia('(max-width: 767px)')
    update()
    mql.addEventListener('change', update)
  })

  onUnmounted(() => {
    mql?.removeEventListener('change', update)
    mql = null
  })

  return { isMobile }
}
