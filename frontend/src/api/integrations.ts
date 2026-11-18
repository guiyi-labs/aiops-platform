import { authorizedRequest } from './client'
export interface IntegrationTool { name: string; description: string; method: 'GET'; path: string; arguments: { name: string; required: boolean; description: string }[] }
export interface ToolValidation { allowed: boolean; executed: false; reason?: string; method?: string; path?: string; arguments?: Record<string, string> }
export function listIntegrationTools(token: string) { return authorizedRequest<{ transport: string; read_only: boolean; items: IntegrationTool[] }>('/api/v1/integrations/tools', token) }
export function validateIntegrationTool(token: string, tool: string, args: Record<string, string>) { return authorizedRequest<ToolValidation>('/api/v1/integrations/tools/validate', token, { method: 'POST', body: JSON.stringify({ tool, arguments: args }) }) }
