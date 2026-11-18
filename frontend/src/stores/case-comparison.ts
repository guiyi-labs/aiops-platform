import { ref } from 'vue'
import { defineStore } from 'pinia'
import type { KnowledgeEntry } from '../api/knowledge'

// Keep only the selected context in memory across the shell's query-route
// remount. No persistence, library mutation or model call is involved.
export const useCaseComparisonStore = defineStore('case-comparison', () => {
  const reference = ref<KnowledgeEntry | null>(null)
  function select(entry: KnowledgeEntry) { reference.value = entry }
  return { reference, select }
})
