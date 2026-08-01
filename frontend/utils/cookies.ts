export const ACCESS_COOKIE = 'access_token'
export const REFRESH_COOKIE = 'refresh_token'
export const ACCESS_MAX_AGE = 15 * 60
export const REFRESH_MAX_AGE = 7 * 24 * 3600

export function readCookie(name: string): string | null {
  if (import.meta.server) return null
  const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`))
  return match ? decodeURIComponent(match[1]) : null
}

export function writeCookie(name: string, value: string | null, maxAge: number): void {
  if (import.meta.server) return
  if (value === null) {
    document.cookie = `${name}=; Max-Age=0; Path=/; SameSite=Lax`
    return
  }
  const expires = new Date(Date.now() + maxAge * 1000).toUTCString()
  document.cookie = `${name}=${encodeURIComponent(value)}; Expires=${expires}; Path=/; SameSite=Lax`
}
