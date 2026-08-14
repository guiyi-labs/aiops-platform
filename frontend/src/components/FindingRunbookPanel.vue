<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowRight, ChevronDown, ChevronUp, Route, Sparkles, Stethoscope, Workflow, Zap } from 'lucide-vue-next'
import { getInsightRunbook } from '../api/insight'
import type { InsightRunbook } from '../types/insight'
import { useAuthStore } from '../stores/auth'

// M113-1 finding → runbook preview navigation.
//
// A reusable read-only drill-down: given a posture / optimization finding and
// the cluster it was observed for, resolves the M81 closed-loop runbook
// (deterministic diagnosis routes, corroborating inspection rules, AI
// explanation deep-link, dry-run operation candidates) and renders the steps.
// It never mutates state and operations are previews only (ADR 0004).

const props = defineProps<{
  domain: string
  code?: string
  kind: string
  namespace?: string
  name: string
  clusterId: number | null
}>()

const auth = useAuthStore()

const runbook = ref<InsightRunbook | null>(null)
const loading = ref(false)
const error = ref('')

const expanded = ref(false)
const hasRunbook = computed(() => Boolean(runbook.value))

async function toggle(): Promise<void> {
  if (expanded.value) {
    expanded.value = false
    return
  }
  if (props.clusterId === null) return
  if (!runbook.value) {
    loading.value = true
    error.value = ''
    try {
      runbook.value = await getInsightRunbook(auth.accessToken ?? '', {
        clusterId: props.clusterId,
        domain: props.domain,
        code: props.code ?? '',
        kind: props.kind,
        namespace: props.namespace,
        name: props.name,
      })
    } catch {
      error.value = '闭环洞察加载失败，请稍后重试'
    } finally {
      loading.value = false
    }
  }
  expanded.value = true
}
</script>

<template>
  <div class="finding-runbook">
    <button type="button" class="finding-runbook-toggle" :aria-expanded="expanded" :disabled="loading" @click="toggle">
      <Route :size="14" />
      {{ loading ? '加载中…' : (hasRunbook ? '收起闭环' : '查看闭环') }}
      <ChevronUp v-if="expanded" :size="14" />
      <ChevronDown v-else :size="14" />
    </button>

    <div v-if="expanded && error" class="notice error finding-runbook-error">{{ error }}</div>
    <div v-if="expanded && runbook && !error" class="finding-runbook-steps">
      <div v-for="route in runbook.diagnoses" :key="route.resource_kind" class="finding-runbook-step">
        <span class="finding-runbook-step-icon"><Stethoscope :size="15" /></span>
        <div class="finding-runbook-step-body">
          <p class="finding-runbook-step-title">确定性诊断 · {{ route.resource_kind }}</p>
          <p class="finding-runbook-step-desc">{{ route.summary }}</p>
          <p class="finding-runbook-step-meta">{{ route.rule_ids.join(' / ') }}</p>
        </div>
        <router-link class="finding-runbook-link" to="/diagnoses">去诊断 <ArrowRight :size="13" /></router-link>
      </div>

      <div v-if="runbook.inspection.length" class="finding-runbook-step">
        <span class="finding-runbook-step-icon"><Workflow :size="15" /></span>
        <div class="finding-runbook-step-body">
          <p class="finding-runbook-step-title">巡检佐证 · M52</p>
          <p v-for="rule in runbook.inspection" :key="rule.rule_code" class="finding-runbook-step-meta">{{ rule.signal_code }}</p>
        </div>
        <router-link class="finding-runbook-link" to="/inspection">去巡检 <ArrowRight :size="13" /></router-link>
      </div>

      <div v-if="runbook.ai_explanation" class="finding-runbook-step">
        <span class="finding-runbook-step-icon"><Sparkles :size="15" /></span>
        <div class="finding-runbook-step-body">
          <p class="finding-runbook-step-title">AI 引用解释 · M55</p>
          <p class="finding-runbook-step-desc">{{ runbook.ai_explanation.summary }}</p>
        </div>
        <router-link class="finding-runbook-link" to="/aiops/investigator">AI 调查 <ArrowRight :size="13" /></router-link>
      </div>

      <div v-if="runbook.operations.length" class="finding-runbook-step">
        <span class="finding-runbook-step-icon"><Zap :size="15" /></span>
        <div class="finding-runbook-step-body">
          <p class="finding-runbook-step-title">受控操作预览 · M19（dry-run）</p>
          <p v-for="op in runbook.operations" :key="op.action" class="finding-runbook-step-meta">
            {{ op.action }}{{ op.dry_run_first ? ' · 仅预览' : '' }}
          </p>
        </div>
        <router-link class="finding-runbook-link" to="/diagnoses">操作台 <ArrowRight :size="13" /></router-link>
      </div>
    </div>
  </div>
</template>

<style scoped>
.finding-runbook-toggle {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: 6px;
  padding: 4px 8px;
  border: 1px solid var(--border-subtle);
  background: var(--surface-2);
  color: var(--text-secondary);
  font-size: 11px;
  cursor: pointer;
}
.finding-runbook-toggle:hover { background: var(--surface-3); }
.finding-runbook-error { margin-top: 6px; }
.finding-runbook-steps { margin-top: 6px; display: grid; gap: 6px; }
.finding-runbook-step {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr) max-content;
  gap: 8px;
  align-items: start;
  padding: 7px 9px;
  border: 1px solid var(--border-subtle);
  background: var(--surface-2);
  font-size: 11px;
}
.finding-runbook-step-icon { display: inline-flex; margin-top: 2px; color: var(--text-tertiary); }
.finding-runbook-step-title { margin: 0; font-weight: 650; color: var(--text-primary); }
.finding-runbook-step-desc { margin: 2px 0 0; color: var(--text-secondary); }
.finding-runbook-step-meta { margin: 2px 0 0; color: var(--text-tertiary); }
.finding-runbook-link { display: inline-flex; align-items: center; gap: 3px; color: var(--accent); white-space: nowrap; }
@media (max-width: 640px) { .finding-runbook-step { grid-template-columns: 1fr; } .finding-runbook-link { margin-top: 3px; } }
</style>
