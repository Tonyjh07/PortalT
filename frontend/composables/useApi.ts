import { $fetch, type FetchOptions } from 'ofetch'
import type { ApiResponse } from '~/types'

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
  return api<ApiResponse<T>>(request, options)
}
