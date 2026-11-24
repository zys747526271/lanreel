import { request } from './client'
import type { TokenPair, User } from './types'

export interface AuthResult {
  user: User
  tokens: TokenPair
}

export function login(username: string, password: string) {
  return request<{ data: AuthResult }>('/auth/login', {
    method: 'POST',
    body: { username, password },
    auth: false,
    noRetry: true,
  })
}

export function register(username: string, password: string) {
  return request<{ data: AuthResult }>('/auth/register', {
    method: 'POST',
    body: { username, password },
    auth: false,
    noRetry: true,
  })
}

export function logout() {
  return request<{ data: { success: boolean } }>('/auth/logout', { method: 'POST' })
}

export function me() {
  return request<{ data: { user: User; registration_open: boolean } }>('/auth/me')
}
