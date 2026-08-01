export type Role = 'admin' | 'user' | 'viewer'

export type VMStatus = 'poweredOn' | 'poweredOff' | 'suspended' | 'unknown'

export interface User {
  id: string
  username: string
  email: string
  role: Role
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

export interface Plugin {
  id: string
  name: string
  icon: string
  route: string
  iframe_url: string
  permission: string
  sort_order: number
  is_active: boolean
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
