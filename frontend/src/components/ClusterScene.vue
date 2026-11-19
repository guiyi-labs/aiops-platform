<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { ArrowUpRight, Box, Cpu, Expand, Focus, HardDrive, MousePointer2, Orbit, Pause, Play, RotateCcw, Server, ShieldCheck } from 'lucide-vue-next'
import type { NodeMetric, NodeResource, Pod } from '../types/kubernetes'
import { cpuMillicores, formatCPU, formatMemory, memoryBytes, utilizationPercent } from '../utils/resource-metrics'
import { podHealth } from '../utils/resource-health'

const props = defineProps<{
  clusterId: number
  clusterName: string
  nodes: NodeResource[] | null
  pods: Pod[] | null
  metrics: NodeMetric[] | null
  nodeCount: number | null
  expanded: boolean
  loading: boolean
}>()
const emit = defineEmits<{ expand: [event: MouseEvent] }>()
const id = useId().replace(/[^a-zA-Z0-9-]/g, '')
const sceneElement = ref<HTMLElement | null>(null)
const selectedName = ref('')
const podSearch = ref('')
const onlyIssues = ref(false)
const selectedPod = ref('')
const yaw = ref(.28)
const pitch = ref(.54)
const autoRotate = ref(false)
const touring = ref(false)
const paused = ref(false)
const reducedMotion = ref(false)
const inView = ref(true)
const pageVisible = ref(!document.hidden)
const dragging = ref(false)
let frame = 0
let lastFrame = 0
let lastTour = 0
let observer: IntersectionObserver | undefined
let motionQuery: MediaQueryList | undefined
let dragStart = { x: 0, y: 0, yaw: 0, pitch: 0 }

const sceneNodes = computed(() => (props.nodes ?? []).slice(0, 6).map((node, index, items) => {
  const angle = -Math.PI / 2 + index * 2 * Math.PI / items.length
  const ready = node.status.conditions.find(item => item.type === 'Ready')?.status
  const metric = props.metrics?.find(item => item.metadata.name === node.metadata.name)
  const cpu = cpuMillicores(metric?.usage.cpu ?? '')
  const memory = memoryBytes(metric?.usage.memory ?? '')
  const allocCPU = cpuMillicores(node.status.allocatable?.cpu ?? '')
  const allocMemory = memoryBytes(node.status.allocatable?.memory ?? '')
  const pods = props.pods?.filter(item => item.spec.nodeName === node.metadata.name) ?? null
  const issues = pods?.filter(item => ['warning', 'critical'].includes(podHealth(item))).length ?? null
  const control = Object.keys(node.metadata.labels ?? {}).some(label => ['node-role.kubernetes.io/control-plane', 'node-role.kubernetes.io/master'].includes(label))
  return { name: node.metadata.name, ready: ready === 'True' ? 'ready' : ready === 'False' ? 'warning' : 'unknown', control,
    x: Math.cos(angle) * 242, z: Math.sin(angle) * 242, pods, issues, cpu, memory,
    cpuPercent: cpu !== null && allocCPU !== null ? utilizationPercent(cpu, allocCPU) : null,
    memoryPercent: memory !== null && allocMemory !== null ? utilizationPercent(memory, allocMemory) : null,
    address: node.status.addresses?.find(item => item.type === 'InternalIP')?.address ?? '地址未返回',
    timestamp: metric?.timestamp, info: node.status.nodeInfo, schedulable: !node.spec.unschedulable,
  }
}))
const active = computed(() => sceneNodes.value.find(node => node.name === selectedName.value) ?? sceneNodes.value[0])
const podKey = (pod: Pod) => `${pod.metadata.namespace ?? ''}/${pod.metadata.name}`
const filteredPods = computed(() => {
  const query = podSearch.value.trim().toLowerCase()
  const priority = { critical: 0, warning: 1, unknown: 2, healthy: 3 }
  return [...(active.value?.pods ?? [])].filter(pod => (!query || podKey(pod).toLowerCase().includes(query)) && (!onlyIssues.value || ['warning', 'critical'].includes(podHealth(pod))))
    .sort((a, b) => priority[podHealth(a)] - priority[podHealth(b)] || podKey(a).localeCompare(podKey(b)))
})
watch(() => [props.clusterId, active.value?.name], () => { podSearch.value = ''; onlyIssues.value = false; selectedPod.value = '' })
watch(filteredPods, pods => { if (!pods.some(pod => podKey(pod) === selectedPod.value)) selectedPod.value = '' })
function podReason(pod: Pod) {
  const statuses = pod.status.containerStatuses ?? []
  return statuses.find(item => item.state.waiting?.reason)?.state.waiting?.reason || pod.status.reason || pod.status.phase || '未知'
}
function podRestarts(pod: Pod) { return (pod.status.containerStatuses ?? []).reduce((sum, item) => sum + item.restartCount, 0) }
function routeForPod(pod: Pod) { return { path: '/workloads', query: { cluster: props.clusterId, kind: 'Pod', namespace: pod.metadata.namespace, name: pod.metadata.name } } }
function togglePod(pod: Pod) { selectedPod.value = selectedPod.value === podKey(pod) ? '' : podKey(pod); touring.value = false }
const readyCount = computed(() => props.nodes?.filter(node => node.status.conditions.some(item => item.type === 'Ready' && item.status === 'True')).length ?? null)
const issueCount = computed(() => props.pods?.filter(pod => ['warning', 'critical'].includes(podHealth(pod))).length ?? null)
const unassigned = computed(() => props.pods?.filter(pod => !pod.spec.nodeName).length ?? null)
const motionRunning = computed(() => !paused.value && !reducedMotion.value && inView.value && pageVisible.value)
watch(() => [props.clusterId, sceneNodes.value.map(node => node.name).join('|')], () => {
  if (!sceneNodes.value.some(node => node.name === selectedName.value)) selectedName.value = sceneNodes.value[0]?.name ?? ''
})

function project(x: number, y: number, z: number) {
  const horizontal = x * Math.cos(yaw.value) - z * Math.sin(yaw.value)
  const depth = x * Math.sin(yaw.value) + z * Math.cos(yaw.value)
  return { x: 500 + horizontal, y: 352 + depth * Math.sin(pitch.value) - y * Math.cos(pitch.value), depth }
}
function point(x: number, y: number, z: number) { const p = project(x, y, z); return `${p.x.toFixed(2)},${p.y.toFixed(2)}` }
function ring(radius: number, height = 0) {
  return Array.from({ length: 97 }, (_, index) => { const angle = index * Math.PI * 2 / 96; return `${index ? 'L' : 'M'}${point(Math.cos(angle) * radius, height, Math.sin(angle) * radius)}` }).join(' ') + ' Z'
}
function cuboid(x: number, z: number, width: number, bottom: number, height: number) {
  const w = width / 2
  const corners = [[x - w, z - w], [x + w, z - w], [x + w, z + w], [x - w, z + w]] as const
  const sides = corners.map(([cx, cz], index) => {
    const next = corners[(index + 1) % 4]
    return { key: index, points: `${point(cx, bottom, cz)} ${point(next[0], bottom, next[1])} ${point(next[0], bottom + height, next[1])} ${point(cx, bottom + height, cz)}`, depth: (project(cx, 0, cz).depth + project(next[0], 0, next[1]).depth) / 2 }
  }).sort((a, b) => a.depth - b.depth)
  return [...sides, { key: 4, points: corners.map(([cx, cz]) => point(cx, bottom + height, cz)).join(' '), depth: 0 }]
}
const grid = computed(() => Array.from({ length: 15 }, (_, index) => {
  const n = (index - 7) * 48
  return [`M${point(n, -12, -336)} L${point(n, -12, 336)}`, `M${point(-336, -12, n)} L${point(336, -12, n)}`]
}).flat())
const objects = computed(() => [
  { name: '__core', x: 0, z: 0, core: true, node: null },
  ...sceneNodes.value.map(node => ({ name: node.name, x: node.x, z: node.z, core: false, node })),
].sort((a, b) => project(a.x, 0, a.z).depth - project(b.x, 0, b.z).depth))
const labels = computed(() => sceneNodes.value.map(node => {
  const position = project(node.x, 0, node.z)
  return { ...node, position: { ...position, y: position.y + (position.depth < -50 ? -185 : 0) } }
}))
function routeForNode(name: string) { return { path: '/workloads', query: { cluster: props.clusterId, kind: 'Node', name } } }
function selectNode(name: string) { selectedName.value = name; touring.value = false }
function resetView() { yaw.value = .28; pitch.value = .54; autoRotate.value = false }
function toggleTour() { lastTour = performance.now(); touring.value = !touring.value }
function startDrag(event: PointerEvent) {
  if (event.button !== 0 || (event.target as Element).closest('[role="button"]')) return
  dragging.value = true
  autoRotate.value = false
  dragStart = { x: event.clientX, y: event.clientY, yaw: yaw.value, pitch: pitch.value }
  ;(event.currentTarget as Element).setPointerCapture(event.pointerId)
}
function moveDrag(event: PointerEvent) {
  if (!dragging.value) return
  const dx = event.clientX - dragStart.x
  const dy = event.clientY - dragStart.y
  yaw.value = dragStart.yaw + dx * .006
  pitch.value = Math.max(.30, Math.min(.85, dragStart.pitch + dy * .003))
}
function endDrag() { dragging.value = false }
function keyView(event: KeyboardEvent) {
  if (event.target !== event.currentTarget) return
  if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) {
    event.preventDefault(); autoRotate.value = false
    if (event.key === 'ArrowLeft') yaw.value -= .15
    if (event.key === 'ArrowRight') yaw.value += .15
    if (event.key === 'ArrowUp') pitch.value = Math.min(.85, pitch.value + .08)
    if (event.key === 'ArrowDown') pitch.value = Math.max(.30, pitch.value - .08)
  }
}
function tick(time: number) {
  const delta = Math.min(time - lastFrame, 80)
  if (time - lastFrame >= 40) {
    lastFrame = time
    if (motionRunning.value) {
      if (autoRotate.value && !dragging.value) yaw.value += delta * .00007
      if (touring.value && sceneNodes.value.length && time - lastTour > 6000) {
        const index = sceneNodes.value.findIndex(node => node.name === active.value?.name)
        selectedName.value = sceneNodes.value[(index + 1) % sceneNodes.value.length].name
        lastTour = time
      }
    } else lastTour = time
  }
  frame = requestAnimationFrame(tick)
}
function updateMotion() { reducedMotion.value = motionQuery?.matches ?? false }
function updateVisibility() { pageVisible.value = !document.hidden }
onMounted(() => {
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)'); updateMotion()
  motionQuery.addEventListener('change', updateMotion)
  document.addEventListener('visibilitychange', updateVisibility)
  observer = new IntersectionObserver(entries => { inView.value = entries[0]?.isIntersecting ?? true })
  if (sceneElement.value) observer.observe(sceneElement.value)
  frame = requestAnimationFrame(tick)
})
onBeforeUnmount(() => { cancelAnimationFrame(frame); observer?.disconnect(); motionQuery?.removeEventListener('change', updateMotion); document.removeEventListener('visibilitychange', updateVisibility) })
</script>

<template>
  <section ref="sceneElement" class="cluster-scene dash-panel" :class="{ 'scene-expanded': expanded, 'scene-paused': !motionRunning, 'scene-dragging': dragging }" aria-label="立体集群场景">
    <header class="scene-heading">
      <div><span class="scene-eyebrow">SPATIAL CLUSTER <i /> 资源空间</span><h2><Orbit :size="17" />集群立体态势</h2></div>
      <div class="scene-heading-actions"><RouterLink :to="{ path: '/diagnoses', query: { cluster: clusterId, view: 'topology' } }" aria-label="查看完整资源拓扑"><ArrowUpRight :size="17" /></RouterLink><button v-if="!expanded" type="button" class="scene-primary" @click="emit('expand', $event)"><Expand :size="14" />立体大屏</button></div>
    </header>
    <div v-if="expanded" class="scene-telemetry" aria-label="场景数据摘要">
      <div><Server :size="17" /><span>节点就绪<strong>{{ readyCount ?? '—' }}<small>/ {{ nodeCount ?? '—' }}</small></strong></span></div>
      <div><Box :size="17" /><span>已读取 Pod<strong>{{ pods?.length ?? '—' }}<small>个</small></strong></span></div>
      <div :class="{ 'has-issues': issueCount }"><ShieldCheck :size="17" /><span>待关注 Pod<strong>{{ issueCount ?? '—' }}<small>个样本</small></strong></span></div>
      <div><Cpu :size="17" /><span>节点指标<strong>{{ metrics?.length ?? '—' }}<small>/ {{ nodes?.length ?? '—' }}</small></strong></span></div>
    </div>
    <div class="scene-body">
      <div class="scene-stage">
        <div class="stage-coordinate" aria-hidden="true"><span>RESOURCE FIELD / {{ String(clusterId).padStart(2, '0') }}</span><i /> {{ clusterName || '等待集群' }}</div>
        <svg class="cluster-map spatial-map" viewBox="0 0 1000 610" role="group" aria-label="立体集群视图，拖动或使用方向键旋转视角" tabindex="0" @pointerdown="startDrag" @pointermove="moveDrag" @pointerup="endDrag" @pointercancel="endDrag" @keydown="keyView">
          <defs>
            <radialGradient :id="`${id}-floor`"><stop offset="0%" stop-color="#16465b" stop-opacity=".8" /><stop offset="100%" stop-color="#0a172b" stop-opacity="0" /></radialGradient>
            <linearGradient :id="`${id}-beam`" x1="0" y1="1" x2="0" y2="0"><stop offset="0%" stop-color="#64e6f0" stop-opacity=".3" /><stop offset="100%" stop-color="#64e6f0" stop-opacity="0" /></linearGradient>
            <filter :id="`${id}-glow`" x="-70%" y="-70%" width="240%" height="240%"><feGaussianBlur stdDeviation="3" /></filter>
          </defs>
          <g aria-hidden="true">
            <ellipse cx="500" cy="360" rx="435" ry="225" :fill="`url(#${id}-floor)`" />
            <path v-for="(line, index) in grid" :key="index" :d="line" class="scene-grid-line" />
            <path :d="ring(340, -10)" class="scene-boundary" /><path :d="ring(305, -10)" class="scene-orbit" />
            <path :d="ring(285, -10)" class="scene-scan" pathLength="100" />
            <g v-for="node in sceneNodes" :key="node.name" :class="['scene-link', node.ready]">
              <path :d="`M${point(0, 1, 0)} L${point(node.x * .5, 1, node.z * .5)} L${point(node.x, 1, node.z)}`" class="scene-link-base" />
              <path :d="`M${point(0, 1, 0)} L${point(node.x, 1, node.z)}`" class="scene-link-flow" pathLength="100" :style="{ animationDelay: `${-sceneNodes.indexOf(node)}s` }" />
            </g>
            <g v-for="object in objects" :key="object.name" :class="['scene-object', object.core ? 'scene-core' : object.node?.ready, { selected: object.name === active?.name }]">
              <template v-if="object.core">
                <path :d="ring(105, 0)" class="core-ground-glow" :filter="`url(#${id}-glow)`" />
                <polygon v-for="face in cuboid(0, 0, 146, -14, 20)" :key="`base-${face.key}`" :points="face.points" :class="['core-base', `face-${face.key}`]" />
                <polygon :points="`${point(-50, 0, 0)} ${point(50, 0, 0)} ${point(40, 210, 0)} ${point(-40, 210, 0)}`" :fill="`url(#${id}-beam)`" />
                <path :d="ring(81, 14)" class="core-ring" /><path :d="ring(60, 28)" class="core-ring faint" />
                <g class="core-float"><polygon v-for="face in cuboid(0, 0, 63, 65, 64)" :key="`core-${face.key}`" :points="face.points" :class="['core-cube', `face-${face.key}`]" /></g>
                <path :d="ring(38, 148)" class="core-crown" />
                <text :x="project(0, 0, 0).x" :y="project(0, 0, 0).y + 75" class="core-label" text-anchor="middle">KUBERNETES</text>
                <text :x="project(0, 0, 0).x" :y="project(0, 0, 0).y + 96" class="core-sublabel" text-anchor="middle">集群资源中枢</text>
              </template>
              <template v-else-if="object.node">
                <path :d="`M${point(object.x, 0, object.z)} L${point(object.x, 165, object.z)}`" class="node-beacon" />
                <polygon v-for="face in cuboid(object.x, object.z, 105, -8, 16)" :key="`plinth-${face.key}`" :points="face.points" :class="['node-plinth', `face-${face.key}`]" />
                <g v-for="tier in 3" :key="tier"><polygon v-for="face in cuboid(object.x, object.z, 49, 14 + (tier - 1) * 25, 20)" :key="face.key" :points="face.points" :class="['server-face', `face-${face.key}`]" /></g>
                <circle :cx="project(object.x, 93, object.z).x" :cy="project(object.x, 93, object.z).y" r="4" class="node-status-light" />
                <g v-for="(pod, index) in object.node.pods?.slice(0, 18) ?? []" :key="pod.metadata.uid || `${pod.metadata.namespace}/${pod.metadata.name}`" :class="['pod-cell', podHealth(pod)]">
                  <polygon v-for="face in cuboid(object.x + Math.cos(index / Math.min(object.node.pods?.length ?? 1, 18) * 2 * Math.PI) * 48, object.z + Math.sin(index / Math.min(object.node.pods?.length ?? 1, 18) * 2 * Math.PI) * 48, 9, 12, 9)" :key="face.key" :points="face.points" />
                </g>
                <circle :cx="project(object.x, 104, object.z).x" :cy="project(object.x, 104, object.z).y" r="11" class="node-signal" />
              </template>
            </g>
          </g>
          <g v-for="node in labels" :key="node.name" class="spatial-node-label" :class="[node.ready, { selected: active?.name === node.name }]" role="button" tabindex="0" :aria-label="`查看节点 ${node.name}，${node.ready === 'ready' ? 'Ready' : node.ready === 'warning' ? '未就绪' : '状态未知'}`" :aria-pressed="active?.name === node.name" @click="selectNode(node.name)" @keydown.enter.prevent="selectNode(node.name)" @keydown.space.prevent="selectNode(node.name)">
            <polygon v-for="face in cuboid(node.x, node.z, 64, 0, 105)" :key="face.key" :points="face.points" class="node-hit-area" />
            <rect :x="node.position.x - 98" :y="node.position.y + 29" width="196" height="58" rx="7" />
            <circle :cx="node.position.x - 82" :cy="node.position.y + 46" r="3" />
            <text :x="node.position.x - 72" :y="node.position.y + 51" class="node-label-name">{{ node.name.length > 23 ? node.name.slice(0, 21) + '…' : node.name }}</text>
            <text :x="node.position.x - 81" :y="node.position.y + 72" class="node-label-detail">{{ node.control ? '控制平面' : '工作节点' }} · {{ node.pods?.length ?? '—' }} Pod{{ node.issues ? ` · ${node.issues} 待关注` : '' }}</text>
            <title>{{ node.name }} · {{ node.ready === 'ready' ? 'Ready' : node.ready === 'warning' ? '未就绪' : '状态未知' }}</title>
          </g>
          <text v-if="!sceneNodes.length" x="500" y="540" class="scene-empty" text-anchor="middle">{{ loading ? '正在读取集群节点…' : nodes === null ? '节点数据暂不可用' : '当前集群暂无节点' }}</text>
        </svg>
        <div class="scene-legend"><span><i />Ready</span><span><i class="warning" />待关注</span><span><i class="unknown" />未知</span><span class="scene-count">展示 {{ sceneNodes.length }} / {{ nodeCount ?? '—' }} 节点</span></div>
        <div class="scene-controls" role="group" aria-label="立体场景控制">
          <button type="button" :aria-pressed="autoRotate" :disabled="reducedMotion" @click="autoRotate = !autoRotate"><Orbit :size="14" />环绕</button>
          <button type="button" :aria-pressed="touring" :disabled="reducedMotion || !sceneNodes.length" @click="toggleTour"><Focus :size="14" />巡览</button>
          <button type="button" :aria-pressed="paused" :disabled="reducedMotion" @click="paused = !paused"><Play v-if="paused" :size="14" /><Pause v-else :size="14" />{{ paused ? '继续动效' : '暂停动效' }}</button>
          <button type="button" @click="resetView"><RotateCcw :size="14" />复位</button>
        </div>
        <p class="scene-hint"><MousePointer2 :size="12" />{{ reducedMotion ? '已跟随系统减少动态效果' : '拖动改变视角 · 点击节点查看详情' }}</p>
      </div>
      <aside v-if="active" class="scene-inspector" aria-label="选中节点状态">
        <div class="inspector-kicker"><span>NODE INSPECTOR</span><i :class="active.ready" />{{ active.ready === 'ready' ? 'Ready' : active.ready === 'warning' ? '未就绪' : '状态未知' }}</div>
        <h3>{{ active.name }}</h3><p class="node-address">{{ active.control ? '控制平面' : '工作节点' }}<span>{{ active.address }}</span></p>
        <dl class="inspector-node-info"><div><dt>Kubernetes</dt><dd>{{ active.info?.kubeletVersion || '未返回' }}</dd></div><div><dt>调度</dt><dd>{{ active.schedulable ? '可调度' : '已停止调度' }}</dd></div><div><dt>操作系统</dt><dd>{{ active.info?.osImage || '未返回' }}</dd></div><div><dt>容器运行时</dt><dd>{{ active.info?.containerRuntimeVersion || '未返回' }}</dd></div></dl>
        <div class="inspector-metrics">
          <div v-for="metric in [{ label: 'CPU', icon: Cpu, value: active.cpuPercent, usage: active.cpu === null ? '暂无指标' : formatCPU(active.cpu) }, { label: '内存', icon: HardDrive, value: active.memoryPercent, usage: active.memory === null ? '暂无指标' : formatMemory(active.memory) }]" :key="metric.label" class="inspector-meter"><div><span><component :is="metric.icon" :size="13" />{{ metric.label }}</span><strong>{{ metric.value === null ? '—' : `${metric.value}%` }}</strong></div><div class="inspector-track"><i :style="{ width: `${Math.max(0, Math.min(100, metric.value ?? 0))}%` }" /></div><small>{{ metric.usage }}<span>占可分配资源</span></small></div>
        </div>
        <div class="inspector-pods"><span>Pod 样本<strong>{{ active.pods?.length ?? '—' }}</strong></span><span>待关注<strong :class="{ 'has-issues': active.issues }">{{ active.issues ?? '—' }}</strong></span></div>
        <section class="inspector-workloads" aria-label="节点 Pod 列表">
          <div class="workloads-heading"><h4>节点上的 Pod</h4><small aria-live="polite">{{ filteredPods.length }} / {{ active.pods?.length ?? '—' }} 已读取</small></div>
          <div v-if="active.pods?.length" class="workload-filters"><input v-model="podSearch" type="search" aria-label="搜索节点 Pod" placeholder="搜索名称 / 命名空间" @input="touring = false" /><button type="button" :aria-pressed="onlyIssues" @click="onlyIssues = !onlyIssues; touring = false">仅待关注</button></div>
          <ul v-if="filteredPods.length" class="node-pod-list" tabindex="0" aria-label="该节点已读取的 Pod，异常优先">
            <li v-for="pod in filteredPods" :key="podKey(pod)">
              <button type="button" class="node-pod-toggle" :aria-expanded="selectedPod === podKey(pod)" @click="togglePod(pod)"><i :class="podHealth(pod)" /><span class="pod-identity">{{ pod.metadata.name }}<small>{{ pod.metadata.namespace }} · 就绪 {{ pod.status.containerStatuses?.filter(item => item.ready).length ?? 0 }}/{{ pod.spec.containers.length }} · 重启 {{ podRestarts(pod) }}</small></span><b :class="podHealth(pod)">{{ podReason(pod) }}</b></button>
              <div v-if="selectedPod === podKey(pod)" class="node-pod-detail"><p>Pod IP <strong>{{ pod.status.podIP || '未分配' }}</strong></p><p v-if="pod.status.message" class="pod-message">{{ pod.status.message }}</p><div v-for="container in pod.spec.containers" :key="container.name" class="pod-container"><span>{{ container.name }}<b>{{ pod.status.containerStatuses?.find(item => item.name === container.name)?.ready ? 'Ready' : '未就绪' }}</b></span><code>{{ container.image }}</code></div><RouterLink :to="routeForPod(pod)">查看 Pod 详情与日志<ArrowUpRight :size="13" /></RouterLink></div>
            </li>
          </ul>
          <p v-else>{{ active.pods === null ? 'Pod 数据暂不可用' : active.pods.length ? '没有符合筛选条件的 Pod' : '当前样本没有该节点的 Pod' }}</p>
          <p class="workload-sample-note">按节点归属展示已读取的 Pod，包含运行中及其他状态。</p>
        </section>
        <RouterLink :to="routeForNode(active.name)" class="scene-detail-link">查看节点证据<ArrowUpRight :size="14" /></RouterLink>
        <p v-if="expanded && active.timestamp" class="inspector-timestamp">指标源时间 {{ new Date(active.timestamp).toLocaleTimeString('zh-CN', { hour12: false }) }}</p>
      </aside>
    </div>
    <footer class="scene-footnote"><span>连线表示集群归属，流光为展示动效；非网络流量或物理位置。</span><span>Pod 来自已读取样本 · 节点最多展示 6 个 · 每节点最多绘制 18 个 Pod 单元<span v-if="unassigned"> · {{ unassigned }} 个 Pod 尚未分配节点</span></span></footer>
  </section>
</template>

<style scoped>
.cluster-scene { container: cluster-scene / inline-size; }
.cluster-scene { --scene-bg: #081421; --scene-line: #24445a; --scene-muted: #9db6ca; --scene-text: #edf8ff; --scene-cyan: #6be8ed; --scene-green: #7ce6bb; --scene-amber: #ffd08c; position: relative; display: flex; flex-direction: column; padding: 20px; color: var(--scene-text); border-color: #27475c; background: radial-gradient(ellipse at 52% 42%, #103546 0%, #0a1b2c 42%, #071320 100%); overflow: hidden; }
.cluster-scene::before { content: ''; position: absolute; pointer-events: none; inset: 0; background-image: radial-gradient(#77dbe326 .7px, transparent .7px); background-size: 22px 22px; mask-image: linear-gradient(transparent, #0006, transparent); }
.scene-heading { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.scene-heading h2 { color: var(--scene-text); display: flex; align-items: center; gap: 8px; margin: 8px 0 0; font-size: 14px; }
.scene-heading h2 svg { color: var(--scene-cyan); }
.scene-eyebrow { color: var(--scene-muted); font: 9px var(--font-mono); letter-spacing: 1.4px; display: flex; gap: 8px; align-items: center; }
.scene-eyebrow i { width: 3px; height: 3px; background: var(--scene-cyan); }
.scene-heading-actions, .scene-controls { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.scene-heading-actions a { display: grid; place-items: center; width: 32px; height: 32px; color: var(--scene-cyan); }
.cluster-scene button { display: inline-flex; align-items: center; justify-content: center; gap: 5px; padding: 7px 9px; border: 1px solid var(--scene-line); background: #0b2234; border-radius: 6px; color: #bed3e3; min-height: 32px; font-size: 10px; cursor: pointer; }
.cluster-scene button:hover, .cluster-scene button[aria-pressed='true'] { color: var(--scene-cyan); background: #153b49; border-color: #43858c; }
.cluster-scene button.scene-primary { color: var(--scene-cyan); border-color: #3c747e; }
.cluster-scene button:disabled { opacity: .5; cursor: default; }
.cluster-scene :is(button, a, select, [tabindex]):focus-visible { outline: 2px solid var(--scene-cyan); outline-offset: 3px; }
.scene-body { display: grid; min-width: 0; min-height: 0; flex: 1; }
.scene-stage { position: relative; min-width: 0; display: flex; flex-direction: column; justify-content: center; }
.stage-coordinate { display: none; position: absolute; left: 18px; top: 16px; gap: 10px; align-items: center; color: var(--scene-muted); font: 10px var(--font-mono); }
.stage-coordinate span { letter-spacing: 2px; }
.stage-coordinate i { width: 4px; height: 4px; background: var(--scene-cyan); }
.spatial-map { display: block; width: 100%; min-height: 230px; flex: 1; overflow: visible; touch-action: pan-y; cursor: grab; }
.scene-dragging .spatial-map { cursor: grabbing; touch-action: none; }
.scene-grid-line { stroke: #295066; stroke-width: .7; opacity: .5; fill: none; }
.scene-boundary { fill: #0b20372b; stroke: #306379; stroke-width: 1; }
.scene-orbit { fill: none; stroke: #36697a; stroke-dasharray: 2 10; }
.scene-scan { fill: none; stroke: #5bc9d8; stroke-width: 2; stroke-dasharray: 12 88; opacity: .5; animation: scene-scan 18s linear infinite; }
.scene-link-base { fill: none; stroke: #328096; stroke-width: 2; opacity: .5; }
.scene-link-flow { fill: none; stroke: #86f4f2; stroke-width: 3; stroke-dasharray: 3 97; animation: scene-flow 4s linear infinite; }
.scene-link.warning .scene-link-flow { stroke: var(--scene-amber); }
.scene-link.unknown .scene-link-flow { stroke: #a6b5c8; }
.core-base, .node-plinth { stroke: #3c8a9a; stroke-width: 1; fill: #113549; }
.core-base.face-4 { fill: #164757; stroke: #6ce6e8; }
.core-base.face-0, .core-base.face-2 { fill: #102535; }
.core-ground-glow { fill: #51d3de22; stroke: #5fe1e8; stroke-width: 10; }
.core-cube { fill: #163d58; stroke: #85f0f5; stroke-width: 1.5; }
.core-cube.face-4 { fill: #276b7b; }
.core-cube.face-1, .core-cube.face-3 { fill: #134250; }
.core-float { animation: scene-float 5s ease-in-out infinite; }
.core-ring, .core-crown { fill: none; stroke: #8cf6ed; stroke-width: 2; }
.core-ring.faint { opacity: .4; stroke-width: 1; }
.core-crown { opacity: .55; stroke-dasharray: 5 6; }
.core-label { fill: #c3faff; font: 600 14px var(--font-mono); letter-spacing: 3px; }
.core-sublabel { fill: #a0c3d4; font-size: 13px; letter-spacing: 2px; }
.scene-object { --node-light: #75debd; }
.scene-object.warning { --node-light: #ffd08c; }
.scene-object.unknown { --node-light: #a0afc3; }
.node-plinth { fill: #123144; stroke: #34747c; }
.node-plinth.face-4 { fill: #173e4c; stroke: var(--node-light); }
.server-face { fill: #123648; stroke: #428294; stroke-width: 1; }
.server-face.face-4 { fill: #21556a; stroke: var(--node-light); }
.server-face.face-1, .server-face.face-3 { fill: #0d293b; }
.selected .server-face { stroke: var(--scene-cyan); }
.selected .server-face.face-4 { fill: #2a7182; }
.node-beacon { stroke: var(--node-light); stroke-dasharray: 2 5; opacity: .35; }
.node-status-light { fill: var(--node-light); }
.node-signal { fill: none; stroke: var(--node-light); opacity: .5; animation: scene-signal 3.5s ease-out infinite; transform-box: fill-box; transform-origin: center; }
.pod-cell { fill: #6fe4bb; stroke: #b0f8db; stroke-width: .5; }
.pod-cell.warning { fill: #f3bb6c; stroke: #ffe7b0; }
.pod-cell.critical { fill: #f7788a; stroke: #ffc0c7; }
.pod-cell.unknown { fill: #879eb9; stroke: #c4cfe0; }
.spatial-node-label { cursor: pointer; }
.node-hit-area { fill: transparent; stroke: none; pointer-events: all; }
.spatial-node-label rect { fill: #0a1e30f2; stroke: #365a6d; }
.spatial-node-label.selected rect { fill: #10313d; stroke: #75dedf; }
.spatial-node-label circle { fill: var(--scene-green); }
.spatial-node-label.warning circle { fill: var(--scene-amber); }
.spatial-node-label.unknown circle { fill: #a0afc3; }
.spatial-node-label:hover rect, .spatial-node-label:focus-visible rect { stroke: #effaff; stroke-width: 2; }
.node-label-name { fill: #ebf8ff; font: 600 13px var(--font-mono); }
.node-label-detail { fill: #adc8d9; font-size: 12px; }
.scene-empty { fill: var(--scene-muted); font-size: 16px; }
.scene-legend { display: flex; justify-content: center; align-items: center; gap: 12px; flex-wrap: wrap; color: #bed2e2; font-size: 10px; margin-bottom: 12px; }
.scene-legend span { display: inline-flex; align-items: center; gap: 5px; }
.scene-legend i, .inspector-kicker i, .inspector-workloads li > i { width: 5px; height: 5px; flex-shrink: 0; display: inline-block; border-radius: 50%; background: var(--scene-green); }
.scene-legend i.warning, .inspector-kicker i.warning, .inspector-workloads i.warning { background: var(--scene-amber); }
.scene-legend i.unknown, .inspector-kicker i.unknown, .inspector-workloads i.unknown { background: #9caec6; }
.scene-controls { justify-content: center; }
.scene-hint { display: flex; align-items: center; justify-content: center; gap: 5px; color: var(--scene-muted); font-size: 9px; margin: 11px 0 0; }
.scene-inspector { margin-top: 16px; padding: 16px; background: linear-gradient(135deg, #123043ad, #0b1e2ee6); border: 1px solid #315365; border-radius: 10px; position: relative; }
.inspector-kicker { display: flex; align-items: center; gap: 6px; color: #b8d4e4; font: 10px var(--font-mono); }
.inspector-kicker span { margin-right: auto; font-size: 8px; letter-spacing: 1px; }
.scene-inspector h3 { margin: 10px 0 6px; color: #f0faff; font: 600 14px var(--font-mono); overflow-wrap: anywhere; }
.node-address { margin: 0; display: flex; justify-content: space-between; flex-wrap: wrap; gap: 6px; color: #a6c3d6; font-size: 10px; }
.inspector-node-info { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin: 16px 0 0; font-size: 10px; }
.inspector-node-info div { min-width: 0; }
.inspector-node-info dt { color: #a6c3d6; margin-bottom: 4px; }
.inspector-node-info dd { color: #e1f1fa; margin: 0; overflow-wrap: anywhere; }
.inspector-metrics { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; margin-top: 16px; }
.inspector-meter > div:first-child { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.inspector-meter span { display: inline-flex; align-items: center; gap: 5px; color: #b2cbdc; font-size: 10px; }
.inspector-meter strong { color: #c6fbfa; font: 600 14px var(--font-mono); }
.inspector-track { height: 4px; border-radius: 3px; background: #244155; margin: 9px 0; overflow: hidden; }
.inspector-track i { display: block; height: 100%; background: linear-gradient(90deg, #3f8aab, #8df1dc); border-radius: inherit; transition: width .5s; }
.inspector-meter small { display: flex; justify-content: space-between; flex-wrap: wrap; gap: 4px; color: #b4cada; font: 9px var(--font-mono); }
.inspector-meter small span { display: none; font-size: 9px; }
.inspector-pods { display: flex; gap: 24px; align-items: center; margin-top: 14px; font-size: 10px; color: #b2cbdc; }
.inspector-pods span { display: flex; gap: 8px; align-items: center; }
.inspector-pods strong { color: #e6f8ff; font: 14px var(--font-mono); }
.scene-detail-link { display: flex; align-items: center; gap: 8px; justify-content: space-between; margin-top: 16px; color: var(--scene-cyan); font-size: 11px; text-decoration: none; }
.scene-footnote { display: flex; flex-direction: column; gap: 4px; margin-top: 14px; color: #9cb9cc; font-size: 9px; text-align: center; line-height: 1.6; position: relative; }
.scene-telemetry { display: grid; grid-template-columns: repeat(4, 1fr); margin-top: 22px; border-top: 1px solid #2c4f61; border-bottom: 1px solid #2c4f61; background: #0d26384d; }
.scene-telemetry > div { display: flex; gap: 14px; align-items: center; padding: 18px 24px; color: var(--scene-cyan); border-right: 1px solid #2c4f61; }
.scene-telemetry > div:last-child { border: 0; }
.scene-telemetry span { color: #aac6d9; font-size: 11px; }
.scene-telemetry strong { display: block; margin-top: 7px; color: #e9fbff; font: 32px/1 var(--font-mono); }
.scene-telemetry small { font: 11px var(--font-mono); margin-left: 9px; color: #9fbfd2; }
.cluster-scene .has-issues { color: var(--scene-amber); }
.inspector-workloads { margin-top: 26px; padding-top: 18px; border-top: 1px solid #305267; }
.workloads-heading { display: flex; justify-content: space-between; gap: 8px; align-items: center; }
.workloads-heading h4 { margin: 0; font-size: 12px; color: #e1f1fa; }
.workloads-heading small { color: #b0ccde; font-size: 10px; }
.workload-filters { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 12px; }
.workload-filters input { --text-primary: #e1f1fa; --bg-elevated: #0b2234; --border-default: #426779; min-width: 0; flex: 1; width: 120px; border: 1px solid #426779; background: #0b2234; color: #e1f1fa; border-radius: 6px; padding: 8px; font-size: 11px; }
.workload-filters input::placeholder { color: #a6c3d6; }
.workload-filters input:focus-visible { outline: 2px solid var(--scene-cyan); outline-offset: 2px; }
.node-pod-list { list-style: none; padding: 0; margin: 8px 0 0; max-height: 300px; overflow-y: auto; overscroll-behavior: contain; scrollbar-gutter: stable; }
.inspector-workloads li { border-bottom: 1px solid #294557; }
.cluster-scene .node-pod-toggle { width: 100%; display: flex; align-items: flex-start; gap: 8px; padding: 12px 5px; text-align: left; background: transparent; border: 0; border-radius: 3px; }
.node-pod-toggle > i { width: 6px; height: 6px; margin-top: 4px; flex-shrink: 0; border-radius: 50%; background: var(--scene-green); }
.node-pod-toggle .pod-identity { min-width: 0; flex: 1; overflow-wrap: anywhere; color: #e1f1fa; font: 11px var(--font-mono); }
.node-pod-toggle small { display: block; margin-top: 6px; color: #b0ccde; font-size: 10px; line-height: 1.6; }
.node-pod-toggle > b { max-width: 34%; overflow-wrap: anywhere; color: #b7d5e6; font: 10px var(--font-mono); }
.node-pod-toggle > b.critical { color: #ffb1be; }
.node-pod-toggle > b.warning { color: var(--scene-amber); }
.node-pod-toggle i.critical { background: #ff9aa8; }
.node-pod-detail { margin: 0 5px 12px; padding: 10px; border: 1px solid #375a6e; border-radius: 6px; background: #091c2c; font-size: 11px; }
.node-pod-detail p { margin: 0 0 10px; color: #b0ccde; overflow-wrap: anywhere; }
.node-pod-detail strong { color: #e1f1fa; margin-left: 8px; font-weight: 400; }
.pod-container { padding: 8px 0; border-top: 1px solid #294557; }
.pod-container span { display: flex; justify-content: space-between; gap: 8px; color: #e1f1fa; overflow-wrap: anywhere; }
.pod-container b { color: #b0ccde; font-size: 10px; font-weight: 400; }
.pod-container code { display: block; margin-top: 6px; color: #b0ccde; overflow-wrap: anywhere; font-size: 10px; }
.node-pod-detail a { display: flex; justify-content: space-between; gap: 8px; padding-top: 8px; color: var(--scene-cyan); text-decoration: none; }
.inspector-workloads .workload-sample-note { font-size: 9px; color: #a6c3d6; margin-bottom: 0; }
.inspector-workloads > p { font-size: 11px; color: #b7cfdf; line-height: 1.7; }
.inspector-timestamp { font: 9px var(--font-mono); color: #a0c0d4; margin: 12px 0 0; }
@container cluster-scene (min-width: 720px) {
  .cluster-scene:not(.scene-expanded) .scene-body { grid-template-columns: minmax(0, 1fr) minmax(290px, 34%); gap: 20px; height: 600px; flex: none; }
  .cluster-scene:not(.scene-expanded) .scene-stage { min-height: 0; }
  .cluster-scene:not(.scene-expanded) .spatial-map { height: 100%; min-height: 0; }
  .cluster-scene:not(.scene-expanded) .scene-inspector { min-height: 0; overflow-y: auto; overscroll-behavior: contain; scrollbar-gutter: stable; }
  .cluster-scene:not(.scene-expanded) .scene-footnote { flex-direction: row; justify-content: space-between; text-align: left; gap: 16px; }
}
.scene-expanded { padding: 24px 28px; min-height: 680px; }
.scene-expanded .scene-heading h2 { font-size: 20px; margin-top: 10px; letter-spacing: 2px; }
.scene-expanded .scene-body { grid-template-columns: minmax(0, 1fr) minmax(320px, 29%); gap: 24px; align-items: center; }
.scene-expanded .stage-coordinate { display: flex; }
.scene-expanded .scene-inspector { padding: 22px; margin-top: 26px; }
.scene-expanded .inspector-metrics { grid-template-columns: 1fr 1fr; gap: 18px; margin-top: 22px; }
.scene-expanded .inspector-meter small span { display: inline; }
.scene-expanded .scene-inspector h3 { font-size: 16px; }
.scene-expanded .inspector-pods { justify-content: space-between; padding-top: 20px; border-top: 1px solid #305267; margin-top: 24px; }
.scene-expanded .scene-detail-link { margin-top: 24px; }
.scene-expanded .scene-footnote { flex-direction: row; justify-content: space-between; gap: 16px; text-align: left; }
.scene-expanded .spatial-map { min-height: 360px; max-height: calc(100dvh - 330px); }
@media (min-width: 1100px) and (min-height: 680px) {
  .scene-expanded .scene-body { min-height: 0; align-items: stretch; }
  .scene-expanded .scene-stage { min-height: 0; }
  .scene-expanded .spatial-map { height: 100%; min-height: 0; max-height: none; flex: 1; }
  .scene-expanded .scene-inspector { min-height: 0; overflow-y: auto; margin-top: 16px; margin-bottom: 12px; align-self: stretch; }
  .scene-expanded .scene-heading, .scene-expanded .scene-telemetry, .scene-expanded .scene-footnote, .scene-expanded .scene-controls, .scene-expanded .scene-hint, .scene-expanded .scene-legend { flex-shrink: 0; }
}
@media (min-width: 1100px) and (max-height: 850px) {
  .scene-expanded { min-height: 0; padding: 18px 24px; }
  .scene-expanded .scene-telemetry { margin-top: 14px; }
  .scene-expanded .scene-telemetry > div { padding: 12px 20px; }
  .scene-expanded .scene-telemetry strong { font-size: 25px; }
  .scene-expanded .scene-body { align-items: start; }
  .scene-expanded .spatial-map { min-height: 250px; max-height: 370px; }
  .scene-expanded .scene-inspector { padding: 16px; margin-top: 16px; }
  .scene-expanded .inspector-metrics { gap: 14px; margin-top: 18px; }
  .scene-expanded .inspector-workloads { margin-top: 16px; padding-top: 12px; }
}
@keyframes scene-flow { to { stroke-dashoffset: -100; } }
@keyframes scene-scan { to { stroke-dashoffset: -100; } }
@keyframes scene-float { 50% { transform: translateY(-8px); } }
@keyframes scene-signal { 0% { transform: scale(.4); opacity: .8; } 100% { transform: scale(2); opacity: 0; } }
.scene-paused *, .scene-paused *::before { animation-play-state: paused !important; }
@media (max-width: 900px) {
  .scene-expanded .scene-body { grid-template-columns: 1fr; }
  .scene-expanded .inspector-metrics { grid-template-columns: 1fr 1fr; }
  .scene-expanded .scene-inspector { margin-top: 20px; }
  .scene-expanded .spatial-map { max-height: none; min-height: 240px; }
  .scene-expanded .scene-telemetry > div { padding: 14px; gap: 8px; }
  .scene-expanded .stage-coordinate { display: none; }
}
@media (max-width: 600px) {
  .cluster-scene, .scene-expanded { padding: 16px; }
  .scene-eyebrow { font-size: 8px; letter-spacing: .5px; }
  .scene-heading h2 { font-size: 13px; }
  .scene-heading-actions { gap: 1px; }
  .scene-heading-actions button { font-size: 9px; padding: 7px; }
  .scene-heading-actions a { width: 27px; }
  .scene-inspector { padding: 13px; }
  .inspector-metrics { gap: 12px; }
  .scene-count { width: 100%; justify-content: center; }
  .scene-expanded .scene-telemetry { grid-template-columns: 1fr 1fr; }
  .scene-telemetry > div:nth-child(2) { border-right: 0; }
  .scene-expanded .scene-inspector { padding: 16px; }
  .scene-expanded .scene-footnote { flex-direction: column; }
  .scene-expanded .scene-heading h2 { font-size: 17px; }
}
@media (prefers-reduced-motion: reduce) { .cluster-scene *, .cluster-scene *::before { animation: none !important; transition: none !important; } }
</style>
