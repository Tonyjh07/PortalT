import type { LoginResult, RefreshResult, User } from '~/types'

const ACCESS_COOKIE = 'access_token'
const REFRESH_COOKIE = 'refresh_token'

export function useAuth() {
  const accessToken = useCookie<string | null>(ACCESS_COOKIE, {
    maxAge: 900,
    sameSite: 'lax',
    path: '/',
  })
  const refreshToken = useCookie<string | null>(REFRESH_COOKIE, {
    maxAge: 7 * 24 * 3600,
    sameSite: 'lax',
    path: '/',
  })
  const user = useState<User | null>('portalt-user', () => null)

  const isAuthenticated = computed(() => !!accessToken.value)

  async function login(username: string, password: string): Promise<User> {
    const { api } = useApi()
    const res = await api<LoginResult>('/auth/login', {
      method: 'POST',
      body: { username, password },
    })
    accessToken.value = res.access_token
    refreshToken.value = res.refresh_token
    user.value = res.user
    return res.user
  }

  async function refresh(): Promise<boolean> {
    if (!refreshToken.value) return false
    try {
      const { api } = useApi()
      const res = await api<RefreshResult>('/auth/refresh', {
        method: 'POST',
        body: { refresh_token: refreshToken.value },
      })
      accessToken.value = res.access_token
      return true
    } catch {
      return false
    }
  }

  async function fetchMe(): Promise<User | null> {
    try {
      const { api } = useApi()
      const res = await api<User>('/auth/me')
      user.value = res
      return res
    } catch {
      return null
    }
  }

  function logout() {
    accessToken.value = null
    refreshToken.value = null
    user.value = null
  }

  return { token: accessToken, user, isAuthenticated, login, refresh, fetchMe, logout }
}
