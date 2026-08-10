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
    },
  },
  nitro: {
    // 构建时生成 .gz/.br 预压缩产物，node 直连/preview 时 h3 按
    // Accept-Encoding 自动协商，低带宽下 JS/CSS 体积约降 70%。
    compressPublicAssets: { gzip: true, brotli: true },
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
      link: [
        { rel: 'icon', type: 'image/x-icon', href: '/favicon.ico' },
        { rel: 'apple-touch-icon', sizes: '180x180', href: '/apple-touch-icon.png' },
        { rel: 'icon', type: 'image/png', sizes: '192x192', href: '/icon-192.png' },
        { rel: 'icon', type: 'image/png', sizes: '512x512', href: '/icon-512.png' },
      ],
    },
  },
})
