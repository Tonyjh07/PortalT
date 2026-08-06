import type { LoginResult, RefreshResult, User } from '~/types'
import { ACCESS_COOKIE, ACCESS_MAX_AGE, REFRESH_COOKIE, REFRESH_MAX_AGE, readCookie, writeCookie } from '~/utils/cookies'
import { can } from '~/utils/permissions'

export function useAuth() {
  const accessToken = useState<string | null>('portalt-access-token', () => readCookie(ACCESS_COOKIE))
  const refreshToken = useState<string | null>('portalt-refresh-token', () => readCookie(REFRESH_COOKIE))
  const user = useState<User | null>('portalt-user', () => null)
  const perms = useState<string[]>('portalt-perms', () => [])

  const isAuthenticated = computed(() => !!accessToken.value)

  // hasPerm 当前用户是否具备指定权限：权限集合（/auth/me，角色矩阵）优先，
  // 未加载时回退用户内置角色表。
  function hasPerm(perm: string): boolean {
    if (perms.value.length > 0) return perms.value.includes(perm)
    return can(user.value, perm)
  }

  async function login(username: string, password: string): Promise<User> {
    const { api } = useApi()
    const res = await api<LoginResult>('/auth/login', {
      method: 'POST',
      body: { username, password },
    })
    writeCookie(ACCESS_COOKIE, res.access_token, ACCESS_MAX_AGE)
    writeCookie(REFRESH_COOKIE, res.refresh_token, REFRESH_MAX_AGE)
    accessToken.value = res.access_token
    refreshToken.value = res.refresh_token
    user.value = res.user
    perms.value = res.user.permissions ?? []
    // 登录响应不含角色矩阵（矩阵在 /auth/me 装载），补齐
    await fetchMe()
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
      writeCookie(ACCESS_COOKIE, res.access_token, ACCESS_MAX_AGE)
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
      perms.value = res.permissions ?? []
      return res
    } catch {
      return null
    }
  }

  function logout() {
    writeCookie(ACCESS_COOKIE, null, 0)
    writeCookie(REFRESH_COOKIE, null, 0)
    accessToken.value = null
    refreshToken.value = null
    user.value = null
    perms.value = []
  }

  return { token: accessToken, user, perms, isAuthenticated, hasPerm, login, refresh, fetchMe, logout }
}
