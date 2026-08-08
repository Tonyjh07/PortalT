import type { MenuItem, Plugin } from '~/types'

function buildMenuTree(plugins: Plugin[]): MenuItem[] {
  const sorted = [...plugins].sort((a, b) => a.sort_order - b.sort_order)
  const map = new Map<string, MenuItem>()
  const children = new Map<string, MenuItem[]>()

  for (const p of sorted) {
    map.set(p.route, { ...p, children: [] })
  }

  for (const p of sorted) {
    const segments = p.route.split('/').filter(Boolean)
    if (segments.length < 2) continue
    const parentPath = '/' + segments.slice(0, -1).join('/')
    if (!children.has(parentPath)) children.set(parentPath, [])
    children.get(parentPath)!.push(map.get(p.route)!)
  }

  for (const [parentPath, list] of children) {
    if (!map.has(parentPath)) {
      const name = parentPath.split('/').filter(Boolean).pop() || '分组'
      map.set(parentPath, {
        id: `group:${parentPath}`,
        name,
        icon: 'mdi:folder',
        route: parentPath,
        type: 'access',
        iframe_url: '',
        api_url: '',
        endpoints: [],
        caddy_rules: '',
        permission: '',
        sort_order: Math.min(...list.map((i) => i.sort_order)),
        is_active: true,
        status: '',
        manifest_json: '',
        children: [],
      })
    }
    const parent = map.get(parentPath)!
    parent.children = (parent.children || []).concat(list).sort((a, b) => a.sort_order - b.sort_order)
  }

  const roots: MenuItem[] = []
  const seen = new Set<string>()
  for (const p of sorted) {
    if (p.route.split('/').filter(Boolean).length > 1) continue
    roots.push(map.get(p.route)!)
    seen.add(p.route)
  }
  for (const [parentPath, node] of map) {
    if (parentPath.startsWith('group:') && !seen.has(parentPath)) {
      roots.push(node)
      seen.add(parentPath)
    }
  }

  return roots.sort((a, b) => a.sort_order - b.sort_order)
}

export function useMenu() {
  const items = useState<MenuItem[]>('portalt-menu', () => [])
  const loaded = useState<boolean>('portalt-menu-loaded', () => false)

  async function load() {
    const { api } = useApi()
    const res = await api<Plugin[]>('/menu')
    items.value = buildMenuTree(res)
    loaded.value = true
    return items.value
  }

  function findByRoute(path: string): MenuItem | null {
    for (const item of items.value) {
      if (item.route === path) return item
      if (item.children?.length) {
        const child = item.children.find((c) => c.route === path)
        if (child) return child
      }
    }
    return null
  }

  return { items, loaded, load, findByRoute }
}
