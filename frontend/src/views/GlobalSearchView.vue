<script setup lang="ts">
import { AlertTriangle, ArrowUpRight, Boxes, BoxIcon, Check, LoaderCircle, Network, Pencil, Play, RefreshCw, Route, Save, Search, Server, Trash2, X } from 'lucide-vue-next'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { APIError } from '../api/auth'
import { createSavedGlobalSearchFilter, deleteSavedGlobalSearchFilter, listSavedGlobalSearchFilters, searchFleetResources, updateSavedGlobalSearchFilter } from '../api/global-search'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import { useAuthStore } from '../stores/auth'
import type { GlobalSearchItem, GlobalSearchKind, GlobalSearchResponse, SavedGlobalSearchFilter } from '../types/global-search'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const kindOptions: Array<{ kind: GlobalSearchKind; label: string; icon: typeof BoxIcon }> = [
  { kind: 'Pod', label: 'Pod', icon: BoxIcon },
  { kind: 'Deployment', label: 'Deployment', icon: Server },
  { kind: 'Service', label: 'Service', icon: Network },
  { kind: 'Ingress', label: 'Ingress', icon: Route },
]

const query = ref('')
const namespace = ref('')
const selectedKinds = ref<GlobalSearchKind[]>(kindOptions.map((item) => item.kind))
const result = ref<GlobalSearchResponse | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const savedFilters = ref<SavedGlobalSearchFilter[]>([])
const savedFilterLimit = ref(20)
const savedFiltersLoading = ref(false)
const savedFilterBusy = ref<number | 'create' | null>(null)
const savedFilterError = ref('')
const savedFilterNotice = ref('')
const showSaveForm = ref(false)
const saveName = ref('')
const editingFilterID = ref<number | null>(null)
const renameValue = ref('')
let activeController: AbortController | null = null

const resultLabel = computed(() => result.value?.complete ? `${result.value.total} 条结果` : `${result.value?.items.length ?? 0}/${result.value?.total ?? 0} 条已返回`)
const coverageLabel = computed(() => result.value ? `${result.value.clusters_searched}/${result.value.clusters_total}` : '0/0')
const completenessLabel = computed(() => {
  if (!result.value) return ''
  if (result.value.complete) return '完整结果'
  if (result.value.clusters_remaining > 0) return `尚有 ${result.value.clusters_remaining} 个集群未搜索`
  if (result.value.remaining > 0) return `尚有 ${result.value.remaining} 条未返回`
  return '部分集群结果不可用'
})
const savedFilterAtLimit = computed(() => savedFilters.value.length >= savedFilterLimit.value)
const currentQueryValid = computed(() => {
  const term = query.value.trim()
  const targetNamespace = namespace.value.trim()
  return term.length >= 2 && term.length <= 64
    && (!targetNamespace || /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(targetNamespace))
    && targetNamespace.length <= 63
    && selectedKinds.value.length > 0
})

function routeValue(value: unknown): string {
  if (Array.isArray(value)) return String(value[0] ?? '')
  return typeof value === 'string' ? value : ''
}

function toggleKind(kind: GlobalSearchKind) {
  if (selectedKinds.value.includes(kind)) {
    if (selectedKinds.value.length === 1) return
    selectedKinds.value = selectedKinds.value.filter((item) => item !== kind)
  } else {
    selectedKinds.value = kindOptions.map((item) => item.kind).filter((item) => item === kind || selectedKinds.value.includes(item))
  }
}

function validate(): boolean {
  const term = query.value.trim()
  if (term.length < 2 || term.length > 64) {
    errorMessage.value = '资源名称需要 2 至 64 个字符'
    return false
  }
  if (namespace.value.length > 63 || (namespace.value && !/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(namespace.value))) {
    errorMessage.value = 'Namespace 格式不正确'
    return false
  }
  return true
}

async function runSearch(updateRoute = true) {
  query.value = query.value.trim()
  namespace.value = namespace.value.trim()
  errorMessage.value = ''
  if (!validate()) return
  activeController?.abort()
  const controller = new AbortController()
  activeController = controller
  loading.value = true
  try {
    if (updateRoute) {
      await router.replace({
        path: '/search',
        query: {
          q: query.value,
          ...(namespace.value ? { namespace: namespace.value } : {}),
          kinds: selectedKinds.value.join(','),
        },
      })
    }
    const response = await searchFleetResources(auth.accessToken, {
      query: query.value,
      namespace: namespace.value || undefined,
      kinds: selectedKinds.value,
      clusterLimit: 20,
      limit: 50,
    }, controller.signal)
    if (activeController === controller) result.value = response
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return
    if (activeController === controller) errorMessage.value = '全局搜索暂时不可用，请稍后重试'
  } finally {
    if (activeController === controller) {
      activeController = null
      loading.value = false
    }
  }
}

function openResult(item: GlobalSearchItem) {
  void router.push({
    path: '/workloads',
    query: {
      cluster: String(item.cluster_id),
      kind: item.kind,
      namespace: item.namespace,
      name: item.name,
    },
  })
}

function healthLabel(item: GlobalSearchItem): string {
  return item.health === 'healthy' ? '健康' : item.health === 'degraded' ? '异常' : '状态信息'
}

function savedFilterErrorMessage(error: unknown, fallback: string): string {
  if (!(error instanceof APIError)) return fallback
  if (error.code === 'SAVED_FILTER_NAME_EXISTS') return '筛选器名称已存在，请使用其他名称。'
  if (error.code === 'SAVED_FILTER_LIMIT_REACHED') return '已达到 20 条保存上限，请先删除不再使用的筛选器。'
  if (error.code === 'SAVED_FILTER_NOT_FOUND') return '该筛选器已不存在，列表已刷新。'
  return fallback
}

function clearSavedFilterFeedback() {
  savedFilterError.value = ''
  savedFilterNotice.value = ''
}

async function loadSavedFilters() {
  savedFiltersLoading.value = true
  savedFilterError.value = ''
  try {
    const response = await listSavedGlobalSearchFilters(auth.accessToken)
    savedFilters.value = response.items
    savedFilterLimit.value = response.limit
  } catch {
    savedFilterError.value = '保存的筛选器暂时无法加载。'
  } finally {
    savedFiltersLoading.value = false
  }
}

function openSaveForm() {
  clearSavedFilterFeedback()
  if (!currentQueryValid.value || savedFilterAtLimit.value) return
  saveName.value = ''
  showSaveForm.value = true
}

async function saveCurrentFilter() {
  clearSavedFilterFeedback()
  const name = saveName.value.trim()
  if (!currentQueryValid.value) {
    savedFilterError.value = '请先填写有效的搜索条件。'
    return
  }
  if (Array.from(name).length < 1 || Array.from(name).length > 40) {
    savedFilterError.value = '筛选器名称需要 1 至 40 个字符。'
    return
  }
  savedFilterBusy.value = 'create'
  try {
    const item = await createSavedGlobalSearchFilter(auth.accessToken, {
      name,
      query: query.value.trim(),
      namespace: namespace.value.trim(),
      kinds: selectedKinds.value,
    })
    savedFilters.value.push(item)
    showSaveForm.value = false
    saveName.value = ''
    savedFilterNotice.value = `已保存“${item.name}”。`
  } catch (error) {
    savedFilterError.value = savedFilterErrorMessage(error, '保存筛选器失败，请稍后重试。')
  } finally {
    savedFilterBusy.value = null
  }
}

async function applySavedFilter(item: SavedGlobalSearchFilter) {
  if (!item.compatible) return
  clearSavedFilterFeedback()
  query.value = item.query
  namespace.value = item.namespace ?? ''
  selectedKinds.value = kindOptions.map((option) => option.kind).filter((kind) => item.kinds.includes(kind))
  await runSearch()
}

function startRename(item: SavedGlobalSearchFilter) {
  clearSavedFilterFeedback()
  editingFilterID.value = item.id
  renameValue.value = item.name
}

async function renameSavedFilter(item: SavedGlobalSearchFilter) {
  const name = renameValue.value.trim()
  clearSavedFilterFeedback()
  if (Array.from(name).length < 1 || Array.from(name).length > 40) {
    savedFilterError.value = '筛选器名称需要 1 至 40 个字符。'
    return
  }
  savedFilterBusy.value = item.id
  try {
    const updated = await updateSavedGlobalSearchFilter(auth.accessToken, item.id, { name })
    savedFilters.value = savedFilters.value.map((entry) => entry.id === item.id ? updated : entry)
    editingFilterID.value = null
    savedFilterNotice.value = `已重命名为“${updated.name}”。`
  } catch (error) {
    savedFilterError.value = savedFilterErrorMessage(error, '重命名失败，请稍后重试。')
  } finally {
    savedFilterBusy.value = null
  }
}

async function overwriteSavedFilter(item: SavedGlobalSearchFilter) {
  clearSavedFilterFeedback()
  if (!currentQueryValid.value) {
    savedFilterError.value = '请先填写有效的搜索条件。'
    return
  }
  savedFilterBusy.value = item.id
  try {
    const updated = await updateSavedGlobalSearchFilter(auth.accessToken, item.id, {
      query: query.value.trim(),
      namespace: namespace.value.trim(),
      kinds: selectedKinds.value,
    })
    savedFilters.value = savedFilters.value.map((entry) => entry.id === item.id ? updated : entry)
    savedFilterNotice.value = `已用当前条件覆盖“${updated.name}”。`
  } catch (error) {
    savedFilterError.value = savedFilterErrorMessage(error, '覆盖筛选器失败，请稍后重试。')
  } finally {
    savedFilterBusy.value = null
  }
}

async function removeSavedFilter(item: SavedGlobalSearchFilter) {
  if (!window.confirm(`确认删除筛选器“${item.name}”？`)) return
  clearSavedFilterFeedback()
  savedFilterBusy.value = item.id
  try {
    await deleteSavedGlobalSearchFilter(auth.accessToken, item.id)
    savedFilters.value = savedFilters.value.filter((entry) => entry.id !== item.id)
    if (editingFilterID.value === item.id) editingFilterID.value = null
    savedFilterNotice.value = `已删除“${item.name}”。`
  } catch (error) {
    savedFilterError.value = savedFilterErrorMessage(error, '删除筛选器失败，请稍后重试。')
    if (error instanceof APIError && error.code === 'SAVED_FILTER_NOT_FOUND') await loadSavedFilters()
  } finally {
    savedFilterBusy.value = null
  }
}

function incompatibilityLabel(item: SavedGlobalSearchFilter): string {
  return item.incompatibility_code === 'SCHEMA_VERSION' ? '版本不兼容' : '查询条件不兼容'
}

onMounted(() => {
  query.value = routeValue(route.query.q)
  namespace.value = routeValue(route.query.namespace)
  const routeKinds = [...new Set(routeValue(route.query.kinds).split(',').filter((item): item is GlobalSearchKind => kindOptions.some((option) => option.kind === item)))]
  if (routeKinds.length > 0) selectedKinds.value = kindOptions.map((item) => item.kind).filter((item) => routeKinds.includes(item))
  if (query.value.length >= 2) void runSearch(false)
  void loadSavedFilters()
})

onBeforeUnmount(() => activeController?.abort())
</script>

<template>
  <ConsoleLayout eyebrow="多集群资源" title="全局搜索">
    <section class="global-search-command">
      <form class="global-search-form" @submit.prevent="runSearch()">
        <label class="global-search-field search-term-field">
          <span>资源名称</span>
          <span class="search-input-shell"><Search :size="17" /><input v-model="query" type="search" maxlength="64" autocomplete="off" placeholder="名称关键字" /></span>
        </label>
        <label class="global-search-field namespace-field">
          <span>Namespace</span>
          <input v-model="namespace" type="text" maxlength="63" autocomplete="off" placeholder="全部 Namespace" />
        </label>
        <div class="global-search-kind-field">
          <span>资源类型</span>
          <div class="search-kind-control" role="group" aria-label="资源类型">
            <button v-for="option in kindOptions" :key="option.kind" type="button" :class="{ active: selectedKinds.includes(option.kind) }" :aria-pressed="selectedKinds.includes(option.kind)" @click="toggleKind(option.kind)">
              <component :is="option.icon" :size="15" /><span>{{ option.label }}</span>
            </button>
          </div>
        </div>
        <button class="primary-button global-search-submit" type="submit" :disabled="loading">
          <LoaderCircle v-if="loading" class="spinning" :size="16" /><Search v-else :size="16" />{{ loading ? '搜索中' : '搜索' }}
        </button>
      </form>
      <p v-if="errorMessage" class="error-message">{{ errorMessage }}</p>
    </section>

    <section class="saved-search-panel" aria-label="保存的筛选器">
      <header class="saved-search-heading">
        <div><p class="context-label">SAVED FILTERS</p><h2>保存的筛选器</h2><span>{{ savedFilters.length }}/{{ savedFilterLimit }}</span></div>
        <div class="saved-search-heading-actions">
          <button class="icon-button compact" type="button" title="刷新筛选器" aria-label="刷新筛选器" :disabled="savedFiltersLoading" @click="loadSavedFilters"><RefreshCw :size="15" :class="{ spinning: savedFiltersLoading }" /></button>
          <button class="secondary-button saved-filter-add" type="button" :disabled="!currentQueryValid || savedFilterAtLimit" @click="openSaveForm"><Save :size="15" />保存当前条件</button>
        </div>
      </header>
      <form v-if="showSaveForm" class="saved-filter-create" @submit.prevent="saveCurrentFilter">
        <label><span>筛选器名称</span><input v-model="saveName" maxlength="40" autocomplete="off" autofocus placeholder="例如：生产环境 API" /></label>
        <button class="icon-button compact" type="button" title="取消保存" aria-label="取消保存" :disabled="savedFilterBusy === 'create'" @click="showSaveForm = false"><X :size="15" /></button>
        <button class="primary-button" type="submit" :disabled="savedFilterBusy === 'create'"><LoaderCircle v-if="savedFilterBusy === 'create'" class="spinning" :size="15" /><Check v-else :size="15" />确认保存</button>
      </form>
      <p v-if="savedFilterError" class="saved-filter-feedback error">{{ savedFilterError }}</p>
      <p v-else-if="savedFilterNotice" class="saved-filter-feedback success">{{ savedFilterNotice }}</p>
      <div v-if="savedFilters.length" class="saved-filter-list">
        <article v-for="item in savedFilters" :key="item.id" class="saved-filter-row" :class="{ incompatible: !item.compatible }">
          <div class="saved-filter-primary">
            <form v-if="editingFilterID === item.id" class="saved-filter-rename" @submit.prevent="renameSavedFilter(item)">
              <input v-model="renameValue" maxlength="40" autocomplete="off" aria-label="新的筛选器名称" @keyup.esc="editingFilterID = null" />
              <button class="icon-button compact" type="submit" title="确认重命名" aria-label="确认重命名" :disabled="savedFilterBusy === item.id"><Check :size="15" /></button>
              <button class="icon-button compact" type="button" title="取消重命名" aria-label="取消重命名" :disabled="savedFilterBusy === item.id" @click="editingFilterID = null"><X :size="15" /></button>
            </form>
            <template v-else><strong>{{ item.name }}</strong><span v-if="!item.compatible" class="saved-filter-incompatible"><AlertTriangle :size="13" />{{ incompatibilityLabel(item) }}</span></template>
            <small>{{ item.query }} · {{ item.namespace || '全部 Namespace' }} · {{ item.kinds.join(', ') }}</small>
          </div>
          <div class="saved-filter-actions">
            <button class="icon-button compact" type="button" title="应用并搜索" :aria-label="`应用筛选器 ${item.name}`" :disabled="!item.compatible || savedFilterBusy === item.id" @click="applySavedFilter(item)"><Play :size="15" /></button>
            <button class="icon-button compact" type="button" title="重命名" :aria-label="`重命名筛选器 ${item.name}`" :disabled="savedFilterBusy === item.id" @click="startRename(item)"><Pencil :size="15" /></button>
            <button class="icon-button compact" type="button" title="用当前条件覆盖" :aria-label="`覆盖筛选器 ${item.name}`" :disabled="!currentQueryValid || savedFilterBusy === item.id" @click="overwriteSavedFilter(item)"><RefreshCw :size="15" /></button>
            <button class="icon-button compact danger" type="button" title="删除" :aria-label="`删除筛选器 ${item.name}`" :disabled="savedFilterBusy === item.id" @click="removeSavedFilter(item)"><Trash2 :size="15" /></button>
          </div>
        </article>
      </div>
      <div v-else-if="savedFiltersLoading" class="saved-filter-empty"><LoaderCircle class="spinning" :size="18" /><span>正在加载筛选器</span></div>
      <div v-else class="saved-filter-empty"><Save :size="18" /><span>还没有保存的筛选器</span></div>
    </section>

    <template v-if="result">
      <section class="global-search-summary" aria-label="搜索摘要">
        <div><span>匹配</span><strong>{{ resultLabel }}</strong></div>
        <div><span>集群覆盖</span><strong>{{ coverageLabel }}</strong></div>
        <div><span>返回上限</span><strong>{{ result.limits.max_results }}</strong></div>
        <div><span>单集群预算</span><strong>{{ result.limits.per_cluster_timeout_ms / 1000 }}s</strong></div>
      </section>

      <section v-if="result.failures.length" class="search-failure-band">
        <header><AlertTriangle :size="17" /><strong>部分集群结果不可用</strong></header>
        <div>
          <span v-for="failure in result.failures" :key="`${failure.cluster_id}-${failure.kind}`">{{ failure.cluster_name }} · {{ failure.kind }} · {{ failure.code === 'TIMEOUT' ? '超时' : '读取失败' }}</span>
        </div>
      </section>

      <section class="global-search-results">
        <header class="search-results-heading"><div><p class="context-label">SEARCH RESULTS</p><h2>资源匹配</h2></div><span>{{ completenessLabel }}</span></header>
        <div v-if="result.items.length" class="global-search-table-scroll">
          <table class="global-search-table">
            <thead><tr><th>集群</th><th>类型</th><th>Namespace</th><th>资源</th><th>状态</th><th aria-label="操作" /></tr></thead>
            <tbody>
              <tr v-for="item in result.items" :key="`${item.cluster_id}-${item.kind}-${item.namespace}-${item.name}`">
                <td><span class="cluster-name-cell"><Boxes :size="15" />{{ item.cluster_name }}</span></td>
                <td><span class="resource-kind-label">{{ item.kind }}</span></td>
                <td>{{ item.namespace || 'cluster-scoped' }}</td>
                <td><strong>{{ item.name }}</strong></td>
                <td><span class="search-health" :class="item.health"><i />{{ healthLabel(item) }}</span><small>{{ item.summary }}</small></td>
                <td><button class="icon-button" type="button" title="打开资源详情" aria-label="打开资源详情" @click="openResult(item)"><ArrowUpRight :size="17" /></button></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty-state"><Search :size="28" /><strong>没有匹配资源</strong></div>
      </section>
    </template>
    <div v-else class="global-search-idle"><Search :size="28" /><strong>尚未执行搜索</strong></div>
  </ConsoleLayout>
</template>
