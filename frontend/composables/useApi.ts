import { $fetch, type FetchOptions } from 'ofetch'

let client: ReturnType<typeof $fetch.create> | null = null

export function useApi() {
  const config = useRuntimeConfig()
  const baseURL = config.public.apiBase || '/api/v1'

  if (!client) {
    client = $fetch.create({
      baseURL,
      onRequest({ options }) {
        const { token } = useAuth()
        if (token.value && !options.headers?.Authorization) {
          options.headers = { ...options.headers, Authorization: `Bearer ${token.value}` }
        }
      },
      onResponse({ response }) {
        const body = response._data
        if (body && typeof body === 'object' && 'code' in body && 'data' in body) {
          // 响应体为统一信封：剥掉外层，暴露 data 给调用方。
          // message（如 Caddy reload 告警）以非枚举属性 __message 挂在
          // data 上透传，调用方用 (res as any)?.__message 读取，不污染
          // 原有响应结构（JSON 序列化不输出）。
          const msg = body.message && body.message !== 'success' ? body.message : undefined
          if (msg && (body.data === null || typeof body.data === 'object')) {
            const data: any = body.data ?? {}
            if (!(data instanceof Array)) {
              Object.defineProperty(data, '__message', { value: msg, enumerable: false })
            }
            response._data = data
          } else {
            response._data = body.data
          }
        }
      },
      onResponseError: async ({ response, options, request }) => {
        const code = (response._data as { code?: number })?.code
        const isAuthEndpoint = typeof request === 'string' && request.includes('/auth/')
        const retried = (options as unknown as { _retried?: boolean })._retried
        if (code === 4002 && !isAuthEndpoint && !retried) {
          ;(options as unknown as { _retried?: boolean })._retried = true
          const { refresh, logout } = useAuth()
          if (await refresh()) {
            const { token } = useAuth()
            options.headers = { ...options.headers, Authorization: `Bearer ${token.value}` }
            return $fetch(request, options)
          }
          logout()
        }
      },
    })
  }

  return { api: client, baseURL }
}

export function apiRequest<T>(request: string, options?: FetchOptions) {
  const { api } = useApi()
  return api<T>(request, options)
}
