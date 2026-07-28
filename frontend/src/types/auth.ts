export type PlatformRole =
  | 'system_admin'
  | 'operations_admin'
  | 'security_auditor'
  | 'viewer'

export interface UserProfile {
  id: number
  username: string
  display_name: string
  roles: PlatformRole[]
}

export interface ManagedUser extends UserProfile {
  status: 'active' | 'disabled'
  last_login_at?: string
  created_at: string
  updated_at: string
}

export interface AuthSession {
  access_token: string
  token_type: 'Bearer'
  expires_in: number
  user: UserProfile
}

export interface RefreshSession {
  id: number
  user_agent: string
  ip_address: string
  current: boolean
  created_at: string
  expires_at: string
}

export interface APIErrorBody {
  code: string
  message: string
  request_id: string
}
