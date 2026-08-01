export default defineNuxtRouteMiddleware(() => {
  const { token } = useAuth()
  if (!token.value) {
    return navigateTo('/login')
  }
})
