import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 插件前端：构建产物输出到 ../static（与插件可执行文件同目录，宿主 /native/frpc-admin/ 挂载）。
// base 必须为相对路径：插件被 /native/frpc-admin/ 反代到根路径，绝对 base 会带上前缀破坏资源地址。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  base: './',
  build: {
    outDir: '../static',
    emptyOutDir: true,
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/native': 'http://127.0.0.1:8080',
    },
  },
})
