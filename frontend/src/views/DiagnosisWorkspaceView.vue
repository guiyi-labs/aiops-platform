<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import ConsoleLayout from '../components/ConsoleLayout.vue'
import DiagnosesView from './DiagnosesView.vue'
import TopologyView from './TopologyView.vue'
import CorrelationCasesView from './CorrelationCasesView.vue'
import DiagnosisValidationView from './DiagnosisValidationView.vue'

const route = useRoute()
const tabs = [
  { id: 'records', label: '诊断记录', component: DiagnosesView },
  { id: 'correlation', label: '关联分析', component: CorrelationCasesView },
  { id: 'topology', label: '资源拓扑', component: TopologyView },
  { id: 'validation', label: '规则验证', component: DiagnosisValidationView },
]
const active = computed(() => tabs.find(tab => tab.id === route.query.view) ?? tabs[0]!)
</script>

<template>
  <ConsoleLayout eyebrow="故障诊断" title="故障诊断">
    <nav class="workspace-tabs" aria-label="诊断视图">
      <RouterLink v-for="tab in tabs" :key="tab.id" :to="{ path: '/diagnoses', query: { ...route.query, view: tab.id } }" :class="{ active: active.id === tab.id }" :aria-current="active.id === tab.id ? 'page' : undefined">{{ tab.label }}</RouterLink>
    </nav>
    <component :is="active.component" />
  </ConsoleLayout>
</template>
