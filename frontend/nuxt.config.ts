export default defineNuxtConfig({
  ssr: false,
  devtools: { enabled: true },
  css: ['~/assets/css/main.css'],
  modules: ['@element-plus/nuxt', '~/modules/wsProxy'],
  // 固定监听 127.0.0.1：dev 模式下仅本机访问（cloudflared 同机回连可用）
  devServer: { host: '127.0.0.1', port: 3000 },
  // Cloudflare Tunnel 转发时 Host 头为外部域名，需放行（否则 Vite 返回 403 阻止请求）
  vite: {
    server: {
      allowedHosts: ['demo.tonyjh07.dpdns.org'],
    },
  },
  elementPlus: {
    importStyle: 'css',
    themes: ['dark'],
  },
  runtimeConfig: {
    public: {
      apiBase: '/api/v1',
      // 远程桌面 WebSocket 直连后端地址（nitro 的 routeRules 反代不支持 WS 升级）。
      // 留空表示与页面同源；部署/测试环境经 NUXT_PUBLIC_API_WS_BASE 指定，
      // 如 http://127.0.0.1:8080
      apiWsBase: '',
    },
  },
  nitro: {
    devProxy: {
      '/api': {
        target: 'http://localhost:8080/api',
        changeOrigin: true,
        ws: true,
      },
      // 原生插件内嵌静态前端（后端托管于 /native/<pluginId>/）
      '/native': {
        target: 'http://localhost:8080/native',
        changeOrigin: true,
      },
    },
  },
  // 生产（nuxt preview / 部署）时 /api 与 /native 反代到后端
  routeRules: {
    '/api/**': { proxy: 'http://localhost:8080/api/**' },
    '/native/**': { proxy: 'http://localhost:8080/native/**' },
  },
  app: {
    head: {
      title: 'PortalT - HomeLab 统一门户',
      meta: [
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'description', content: 'HomeLab 虚拟机与插件统一管理门户' },
      ],
    },
  },
})
