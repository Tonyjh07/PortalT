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
    devProxy: {
      '/api': {
        target: 'http://localhost:8080/api',
        changeOrigin: true,
        ws: true,
      },
    },
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
