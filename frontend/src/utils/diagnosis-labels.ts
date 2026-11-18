const names: Record<string, string> = {
  'pod.image_pull_backoff.v1': '镜像拉取失败',
  'pod.crash_loop_backoff.v1': '容器崩溃循环',
  'pod.oom_killed.v1': '容器内存溢出',
  'pod.pending.v1': 'Pod 调度挂起',
  'service.no_ready_endpoints.v1': 'Service 无可用端点',
  'node.not_ready.v1': '节点未就绪',
  'node.pressure.v1': '节点资源压力',
  'node.metric_sustained_breach.v1': '节点指标持续越界',
  'deployment.replicas_unavailable.v1': '工作负载副本不可用',
  'persistentvolumeclaim.pending.v1': '存储卷绑定等待',
  'horizontalpodautoscaler.saturated.v1': '弹性伸缩达到上限',
  'ingress.backend_unavailable.v1': 'Ingress 后端不可用',
}
export function diagnosisRuleName(id: string): string { return names[id] ?? id }
