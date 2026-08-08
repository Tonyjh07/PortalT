export type Role = string

export type VMStatus = 'poweredOn' | 'poweredOff' | 'suspended' | 'unknown'

export interface User {
  id: string
  username: string
  email: string
  role: Role
  /** 当前用户权限集合（/auth/me 返回，角色矩阵单一事实来源） */
  permissions?: string[]
}

export interface RoleDefinition {
  id: string
  name: string
  description: string
  permissions: string[]
}

export interface PermissionInfo {
  id: string
  description: string
}

export interface VM {
  id: string
  name: string
  status: VMStatus
  cpu: number
  memory_mb: number
  ip_address: string
  host: string
  metadata: Record<string, unknown>
}

export interface VMStatusResult {
  id: string
  name: string
  status: VMStatus
}

export type PluginType = 'access' | 'native'

export interface PluginEndpoint {
  method: string
  path: string
  name: string
  description: string
}

export interface Plugin {
  id: string
  name: string
  icon: string
  route: string
  type: PluginType
  iframe_url: string
  api_url: string
  endpoints: PluginEndpoint[]
  caddy_rules: string
  permission: string
  sort_order: number
  is_active: boolean
  /** native 插件运行态（running/stopped/error/missing），access 恒为空 */
  status: string
  /** native 插件 manifest 缓存 */
  manifest_json: string
}

export interface MenuItem extends Plugin {
  children?: MenuItem[]
}

export interface LoginResult {
  access_token: string
  refresh_token: string
  expires_in: number
  user: User
}

export interface RefreshResult {
  access_token: string
  expires_in: number
}

export interface ApiResponse<T> {
  code: number
  message: string
  data: T
  details?: string
}
