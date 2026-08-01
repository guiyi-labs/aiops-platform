export interface RouteDestinationView {
  host: string
  subset?: string
  weight?: number
}

export interface VirtualServiceView {
  name: string
  namespace: string
  uid: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  created_at: string
  hosts: string[]
  gateways?: string[]
  http_route_count: number
  tcp_route_count: number
  destinations?: RouteDestinationView[]
}

export interface DestinationRuleView {
  name: string
  namespace: string
  uid: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  created_at: string
  host: string
  subset_count: number
  subset_names?: string[]
  traffic_policy_summary?: string
}

export interface ServiceMetric {
  service_name: string
  namespace: string
  request_rate_rps: number
  error_rate_pct: number
  p50_latency_ms: number
  p95_latency_ms: number
  p99_latency_ms: number
  total_requests: number
  total_errors: number
  source_workloads?: string[]
}

export interface TrafficMetrics {
  cluster_id: number
  namespace?: string
  window_start: string
  window_end: string
  services: ServiceMetric[]
  partial?: boolean
}
