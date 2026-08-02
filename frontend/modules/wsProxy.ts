import { defineNuxtModule } from '@nuxt/kit'
import { createProxyServer } from 'httpxy'

export default defineNuxtModule({
  setup(_, nuxt) {
    if (!nuxt.options.dev) return

    const rules = Object.entries(nuxt.options.nitro?.devProxy ?? {}).filter(
      ([, rule]) => typeof rule === 'object' && rule !== null && (rule as { ws?: boolean }).ws,
    )
    if (!rules.length) return

    // nitro 的 devProxy 对 WS 升级不可靠（见 nuxt/cli#107），这里直接代理到后端。
    // 注意：http-proxy 会在 target 路径后前置请求路径，因此只取 target 的 origin，
    // 并保留原始路径转发（与后端路由前缀一致）。
    const proxies = rules.map(([key, rule]) => {
      const target = new URL(String((rule as { target?: string }).target || ''))
      const proxy = createProxyServer({ target: target.origin, changeOrigin: true })
      return [key, proxy] as const
    })

    nuxt.hook('ready', () => {
      const server = nuxt.server as unknown as {
        upgrade?: (req: unknown, socket: unknown, head: unknown) => unknown
      }
      const origUpgrade = server.upgrade?.bind(server)
      server.upgrade = (req: unknown, socket: unknown, head: unknown) => {
        const url = (req as { url?: string }).url ?? ''
        const proxy = proxies.find(([key]) => url.startsWith(key))?.[1]
        if (proxy) {
          proxy.ws(req, socket, head)
          return
        }
        return origUpgrade ? origUpgrade(req, socket, head) : undefined
      }
    })
  },
})
