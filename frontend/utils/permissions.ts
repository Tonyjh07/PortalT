import type { Role, User } from '~/types'

// 内置角色静态权限表（与后端 domain.rolePermissions 对齐）。
// 仅作为后端权限集合（/auth/me 的 permissions）未加载时的兜底；
// 运行时一律以用户权限集合为单一事实来源。
const ROLE_PERMS: Record<string, string[]> = {
  admin: ['vm:view', 'vm:start', 'vm:stop', 'vm:restart', 'vm:manage', 'vm:console', 'plugin:view', 'plugin:manage', 'user:manage'],
  user: ['vm:view', 'vm:start', 'vm:stop', 'vm:restart', 'vm:console', 'plugin:view'],
  viewer: ['vm:view'],
}

export function can(user: User | null, perm: string): boolean {
  if (!user) return false
  if (Array.isArray(user.permissions)) {
    return user.permissions.includes(perm)
  }
  return ROLE_PERMS[user.role]?.includes(perm) ?? false
}

// canWith 显式传入权限集合的判定（调用方持有独立 perms 引用时使用）。
export function canWith(perms: string[] | null | undefined, perm: string): boolean {
  return Array.isArray(perms) && perms.includes(perm)
}

// rolePerms 返回内置角色的静态权限（供兜底与调试展示）。
export function rolePerms(role: Role): string[] {
  return ROLE_PERMS[role] ?? []
}
