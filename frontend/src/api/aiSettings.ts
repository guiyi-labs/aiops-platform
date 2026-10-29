import { authorizedRequest } from './client'

/** 请求协议：responses = OpenAI Responses API；chat_completions = 传统 Chat Completions。 */
export type AIProtocol = 'responses' | 'chat_completions'

/** 解释服务的实时运行状态（由后端 /ai/status 的结构裁剪而来）。 */
export interface AIRuntimeStatus {
  enabled: boolean
  available: boolean
  provider: string
  model: string
  max_concurrent_requests: number
  active_requests: number
  max_output_tokens: number
  daily_token_budget: number
  used_tokens_today: number
  reserved_tokens: number
  remaining_tokens?: number
  explanation_count_today: number
  last_success_at?: string
}

export interface AISettings {
  enabled: boolean
  protocol: AIProtocol
  base_url: string
  model: string
  /** 后端只回「是否已配置」，永不回显密钥明文。 */
  api_key_set: boolean
  api_key_hint: string
  request_timeout_seconds: number
  max_output_tokens: number
  daily_token_budget: number
  max_concurrent_requests: number
  updated_at?: string
  runtime?: AIRuntimeStatus
}

export interface AISettingsUpdate {
  enabled: boolean
  protocol: AIProtocol
  base_url: string
  model: string
  /** 留空表示沿用已保存的密钥。 */
  api_key: string
  /** true 时清除已保存的密钥（仅在未启用状态下允许）。 */
  clear_api_key?: boolean
  request_timeout_seconds: number
  max_output_tokens: number
  daily_token_budget: number
  max_concurrent_requests: number
}

export interface AIProbeRequest {
  protocol: AIProtocol
  base_url: string
  model: string
  api_key: string
  request_timeout_seconds: number
}

export interface AIProbeResult {
  ok: boolean
  detail: string
  probe?: {
    ok: boolean
    method: string
    latency_ms: number
    detail?: string
    http_status?: number
  }
}

export function getAISettings(token: string): Promise<AISettings> {
  return authorizedRequest('/api/v1/settings/ai', token)
}

export function updateAISettings(token: string, body: AISettingsUpdate): Promise<AISettings> {
  return authorizedRequest('/api/v1/settings/ai', token, { method: 'PUT', body: JSON.stringify(body) })
}

export function testAISettings(token: string, body: AIProbeRequest): Promise<AIProbeResult> {
  return authorizedRequest('/api/v1/settings/ai/test', token, { method: 'POST', body: JSON.stringify(body) })
}
