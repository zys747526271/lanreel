/** 首次部署初始化接口（未登录可访问，仅在实例未初始化时生效）。 */
import { request } from './client'
import type { TokenPair, User } from './types'

export interface SetupStatus {
  setup_required: boolean
  version: string
}

/** 查询实例是否仍需初始化。 */
export async function fetchSetupStatus(): Promise<SetupStatus> {
  const res = await request<{ data: SetupStatus }>('/setup/status', {
    auth: false,
    noRetry: true,
    quiet: true,
  })
  return res.data
}

/** 创建第一个管理员，成功后返回令牌（自动登录）。 */
export async function createInitialAdmin(username: string, password: string) {
  const res = await request<{ data: { user: User; tokens: TokenPair } }>('/setup/admin', {
    method: 'POST',
    body: { username, password },
    auth: false,
    noRetry: true,
  })
  return res.data
}
