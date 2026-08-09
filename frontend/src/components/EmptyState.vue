<script setup lang="ts">
import { Inbox } from 'lucide-vue-next'

withDefaults(defineProps<{
  title: string
  description?: string
}>(), {
  description: '',
})
</script>

<template>
  <div class="empty-state" role="status">
    <div class="empty-icon">
      <Inbox :size="26" />
    </div>
    <p class="empty-title">{{ title }}</p>
    <p v-if="description" class="empty-desc">{{ description }}</p>
    <slot />
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 40px 24px;
  text-align: center;
  color: var(--text-secondary);
  animation: empty-in 0.5s cubic-bezier(0.22, 1, 0.36, 1) both;
}
.empty-icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  border-radius: 16px;
  color: var(--blue-500);
  background: color-mix(in srgb, var(--blue-100) 60%, transparent);
  animation: empty-bob 2.6s ease-in-out infinite;
}
.empty-title { margin: 0; font-size: 14px; font-weight: 600; color: var(--text-primary); }
.empty-desc { margin: 0; font-size: 12px; line-height: 1.5; max-width: 320px; }

@keyframes empty-in {
  from { opacity: 0; transform: translateY(10px) scale(0.98); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}
@keyframes empty-bob {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-5px); }
}
</style>