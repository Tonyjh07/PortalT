// 插件一律导航到 /plugins<route>，由 pages/plugins/[...slug].vue 渲染
export function pluginNav(route: string): string {
  return route.startsWith('/plugins') ? route : `/plugins${route}`
}
