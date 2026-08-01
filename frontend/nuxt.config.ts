export default defineNuxtConfig({
  ssr: false,
  devtools: { enabled: true },
  css: ['~/assets/css/main.css'],
  modules: ['@element-plus/nuxt'],
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
