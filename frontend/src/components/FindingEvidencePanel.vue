<script setup lang="ts">
import { computed } from 'vue'
import { ChevronDown, ChevronUp, CircleHelp, FileText, Lightbulb, ShieldAlert } from 'lucide-vue-next'
import type { FindingDetailV2, FindingEvidenceRef, FindingRecommendation } from '../types/finding'

const props = withDefaults(defineProps<{
  finding: FindingDetailV2
  compact?: boolean
}>(), { compact: false })

const expanded = defineModel<boolean>('expanded', { default: false })
const evidence = computed(() => props.finding.evidence)
const recommendations = computed(() => props.finding.recommendations)

const kindLabels: Record<string, string> = {
  resource_state: '资源状态',
  event: '事件',
  log: '日志',
  alert: '告警',
  change: '变更',
  automation: '自动化',
}

const recommendationLabels: Record<string, string> = {
  advisory: '只读建议',
  controlled_action_available: '受控动作',
  manual_only: '人工处理',
}

function kindLabel(kind: FindingEvidenceRef['kind']): string {
  return kindLabels[kind] ?? kind
}

function recommendationLabel(item: FindingRecommendation): string {
  return recommendationLabels[item.kind] ?? item.kind
}

function timeLabel(value?: string): string {
  if (!value) return '时间未知'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

function toggle(): void {
  expanded.value = !expanded.value
}
</script>

<template>
  <section class="finding-evidence-panel" :class="{ compact }" :aria-label="`证据链 ${finding.code}`">
    <button type="button" class="finding-evidence-toggle" :aria-expanded="expanded" @click="toggle">
      <span class="finding-evidence-heading"><ShieldAlert :size="14" />证据链</span>
      <span class="finding-evidence-count">{{ evidence.length }} 条 · {{ finding.rule.source || finding.rule.framework || '规则来源待补充' }}</span>
      <ChevronUp v-if="expanded" :size="14" />
      <ChevronDown v-else :size="14" />
    </button>
    <div v-if="expanded" class="finding-evidence-body">
      <div class="finding-evidence-rule">
        <code>{{ finding.rule.rule_id || finding.code }}</code>
        <span v-if="finding.origin_ids.length > 1">来源 {{ finding.origin_ids.length }} 条</span>
      </div>
      <ol v-if="evidence.length" class="finding-evidence-list">
        <li v-for="item in evidence" :key="item.id" :class="{ missing: item.missing }">
          <span class="finding-evidence-kind"><FileText :size="12" />{{ kindLabel(item.kind) }}</span>
          <div>
            <strong>{{ item.summary || item.id }}</strong>
            <small v-if="item.source">{{ item.source }}</small>
            <small v-if="item.missing">证据缺失 · {{ item.missing_reason || '对象未报告该状态' }}</small>
          </div>
          <time>{{ timeLabel(item.observed_at) }}</time>
        </li>
      </ol>
      <p v-else class="finding-evidence-empty"><CircleHelp :size="13" />暂无可解析证据引用</p>
      <div v-if="recommendations.length" class="finding-recommendations">
        <span class="finding-recommendations-heading"><Lightbulb :size="13" />建议</span>
        <ul>
          <li v-for="item in recommendations" :key="`${item.kind}-${item.text}`">
            <span :class="`finding-recommendation-kind ${item.kind}`">{{ recommendationLabel(item) }}</span>
            <span>{{ item.text }}</span>
            <code v-if="item.capability">{{ item.capability }}</code>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>

<style scoped>
.finding-evidence-panel { margin-top: 8px; border: 1px solid var(--border-subtle); background: var(--surface-2); }
.finding-evidence-panel.compact { margin-top: 6px; }
.finding-evidence-toggle { width: 100%; min-height: 32px; display: flex; align-items: center; gap: 7px; border: 0; background: transparent; color: var(--text-secondary); padding: 7px 9px; text-align: left; cursor: pointer; }
.finding-evidence-toggle:hover { background: var(--surface-3); }
.finding-evidence-heading { display: inline-flex; align-items: center; gap: 5px; color: var(--text-primary); font-weight: 650; }
.finding-evidence-count { flex: 1; font-size: 11px; color: var(--text-tertiary); }
.finding-evidence-body { padding: 0 9px 9px; }
.finding-evidence-rule { display: flex; gap: 8px; align-items: center; padding: 6px 0; font-size: 11px; color: var(--text-tertiary); }
.finding-evidence-rule code { color: var(--text-secondary); }
.finding-evidence-list { list-style: none; padding: 0; margin: 0; display: grid; gap: 5px; }
.finding-evidence-list li { display: grid; grid-template-columns: max-content minmax(0, 1fr) max-content; gap: 8px; align-items: start; padding: 6px 0; border-top: 1px solid var(--border-subtle); font-size: 11px; }
.finding-evidence-list li.missing { color: var(--status-warning); }
.finding-evidence-kind { display: inline-flex; align-items: center; gap: 4px; color: var(--text-tertiary); white-space: nowrap; }
.finding-evidence-list strong, .finding-evidence-list small { display: block; overflow-wrap: anywhere; }
.finding-evidence-list small { margin-top: 2px; color: var(--text-tertiary); }
.finding-evidence-list time { color: var(--text-tertiary); white-space: nowrap; }
.finding-evidence-empty { display: flex; gap: 5px; align-items: center; margin: 0; padding: 7px 0; color: var(--text-tertiary); font-size: 11px; }
.finding-recommendations { margin-top: 6px; border-top: 1px solid var(--border-subtle); padding-top: 7px; }
.finding-recommendations-heading { display: inline-flex; align-items: center; gap: 5px; font-size: 11px; color: var(--text-secondary); font-weight: 650; }
.finding-recommendations ul { list-style: none; margin: 5px 0 0; padding: 0; display: grid; gap: 5px; }
.finding-recommendations li { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; font-size: 11px; }
.finding-recommendation-kind { padding: 2px 5px; border: 1px solid var(--border-subtle); color: var(--text-secondary); }
.finding-recommendation-kind.controlled_action_available { color: var(--status-warning); border-color: var(--status-warning); }
.finding-recommendation-kind.manual_only { color: var(--status-danger); border-color: var(--status-danger); }
.finding-recommendations code { color: var(--text-tertiary); }
@media (max-width: 640px) { .finding-evidence-list li { grid-template-columns: 1fr; gap: 3px; } .finding-evidence-list time { white-space: normal; } }
</style>
