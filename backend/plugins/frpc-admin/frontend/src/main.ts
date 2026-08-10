import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import App from './App.vue'
import './style.css'

const app = createApp(App)
app.use(ElementPlus, { locale: zhCn })

// 插件前端嵌入门户 /native/frpc-admin/，跟随门户（父页面）的主题 class。
// 门户在 <html> 上通过 toggle dark class 切换亮/暗，我们通过同源 iframe 直接读取。
function syncTheme() {
  try {
    const parentDark = window.parent.document.documentElement.classList.contains('dark')
    document.documentElement.classList.toggle('dark', parentDark)
  } catch {
    // 非嵌入环境直接默认暗色
    document.documentElement.classList.add('dark')
  }
}
syncTheme()
// 监听父页面 html class 变更（门户切换主题时自动同步）
try {
  const target = window.parent.document.documentElement
  const obs = new MutationObserver(() => syncTheme())
  obs.observe(target, { attributes: true, attributeFilter: ['class'] })
} catch {
  // 非 iframe 环境不做监听
}

app.mount('#app')
