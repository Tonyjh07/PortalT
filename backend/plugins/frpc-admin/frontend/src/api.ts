// API 客户端：插件前端以同源 iframe 形态嵌入门户（/native/frpc-admin/），
// 因此直接调门户相对路径；认证用 portal 写入的 access_token cookie（SameSite=Lax）。
export const ACCESS_COOKIE = 'access_token'
export const PLUGIN_ID = 'frpc-admin'

// 插件 API 根路径（经门户原生插件反代，宿主注入 X-PortalT-* 身份头并校验权限）。
export const PLUGIN_API = `/api/v1/plugins/native/${PLUGIN_ID}/api`

function readCookie(name: string): string | null {
  const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`))
  return match ? decodeURIComponent(match[1]) : null
}

// request 统一请求：自动带 Bearer token；解析 {code,data,...} 信封。
export async function request<T>(url: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = { ...(init.headers as Record<string, string>) }
  const token = readCookie(ACCESS_COOKIE)
  if (token) headers.Authorization = `Bearer ${token}`
  if (init.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json'

  const res = await fetch(url, { ...init, headers })
  const text = await res.text()
  let body: unknown = null
  try {
    body = text ? JSON.parse(text) : null
  } catch {
    body = text
  }
  if (!res.ok) {
    const errBody = body as { error?: string; content?: string; format?: string; path?: string }
      | { message?: string } | string | null
    const msg = typeof errBody === 'string'
      ? errBody
      : (errBody as { error?: string })?.error
        ?? (errBody as { message?: string })?.message
        ?? `请求失败（HTTP ${res.status}）`
    const err = new Error(msg) as Error & { detail?: { content?: string; format?: string; path?: string } }
    if (typeof errBody === 'object' && errBody && 'content' in errBody) {
      err.detail = { content: errBody.content, format: errBody.format, path: errBody.path }
    }
    throw err
  }
  // 门户信封 { code, data, message } 剥离；插件自身响应 { ...业务体 } 或 { error }
  if (body && typeof body === 'object' && 'code' in body && 'data' in body) {
    return (body as { data: T }).data
  }
  return body as T
}

// ---- 类型定义（与插件 API 契约对应） ----

export interface VM {
  id: string
  name: string
  status?: string
  ip_address?: string
}

export interface Connection {
  vm_id: string
  vm_name?: string
  host: string
  port: number
  user: string
  password?: string
  sudo_enabled: boolean
  sudo_password?: string
  config_path?: string
  format?: string
  restart_cmd?: string
}

export interface ProbeResult {
  version: string
  config_path: string
  format_hint: string
  raw: string
}

export interface ServerConfig {
  server_addr: string
  server_port: number
  token: string
  extra?: Record<string, unknown>
}

export interface Proxy {
  name: string
  type: string
  local_ip: string
  local_port: number
  remote_port: number
  custom_domains?: string[]
  extra?: Record<string, unknown>
}

export interface FrpcConfig {
  format: string
  server: ServerConfig
  proxies: Proxy[]
}

export interface ConfigResponse {
  content: string
  format: string
  server: ServerConfig
  proxies: Proxy[]
  path: string
}

export interface SaveConfigRequest {
  content?: string
  structured?: FrpcConfig
  format?: string
}

export interface SaveConfigResponse {
  syntax_ok: boolean
  syntax_error?: string
  backup_path?: string
  applied: boolean
  restart_output?: string
  rolled_back: boolean
  rollback_error?: string
  error?: string
}

// ---- API 封装 ----

export function listVMs(): Promise<VM[]> {
  return request<VM[]>('/api/v1/vms')
}

export function listConnections(): Promise<Connection[]> {
  return request<Connection[]>(`${PLUGIN_API}/connections`)
}

export function saveConnection(vmId: string, conn: Partial<Connection>): Promise<Connection> {
  return request<Connection>(`${PLUGIN_API}/connections/${encodeURIComponent(vmId)}`, {
    method: 'PUT',
    body: JSON.stringify(conn),
  })
}

export function probe(vmId: string): Promise<ProbeResult> {
  return request<ProbeResult>(`${PLUGIN_API}/vms/${encodeURIComponent(vmId)}/probe`)
}

export function getConfig(vmId: string): Promise<ConfigResponse> {
  return request<ConfigResponse>(`${PLUGIN_API}/vms/${encodeURIComponent(vmId)}/config`)
}

export function saveConfig(vmId: string, req: SaveConfigRequest): Promise<SaveConfigResponse> {
  return request<SaveConfigResponse>(`${PLUGIN_API}/vms/${encodeURIComponent(vmId)}/config`, {
    method: 'PUT',
    body: JSON.stringify(req),
  })
}
