import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import App from './App.vue'
import './style.css'

const app = createApp(App)
app.use(ElementPlus, { locale: zhCn })
// 插件前端嵌入门户 /native/frpc-admin/，门户整体使用暗色主题，这里跟随
// html.dark 类（门户 app.vue 通过 useTheme 写入）。无 dark 类时默认暗色兜底。
document.documentElement.classList.add('dark')
app.mount('#app')
