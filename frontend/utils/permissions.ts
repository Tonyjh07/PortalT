import type { Role, User } from '~/types'

const ROLE_PERMS: Record<Role, string[]> = {
  admin: ['vm:start', 'vm:stop', 'vm:restart', 'plugin:view', 'plugin:manage', 'user:manage'],
  user: ['vm:start', 'vm:stop', 'vm:restart', 'plugin:view'],
  viewer: [],
}

export function can(user: User | null, perm: string): boolean {
  if (!user) return false
  return ROLE_PERMS[user.role]?.includes(perm) ?? false
}
