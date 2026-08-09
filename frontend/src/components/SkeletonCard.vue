<script setup lang="ts">
withDefaults(defineProps<{
  lines?: number
  width?: string
  height?: string
}>(), {
  lines: 1,
  width: '100%',
  height: '100%',
})
</script>

<template>
  <div class="skeleton" :style="{ width, height }" :aria-hidden="true">
    <div
      v-for="n in lines"
      :key="n"
      class="skeleton-line"
      :style="{ width: `calc(${100 - (n - 1) * 22}% - ${(n - 1) * 12}px)` }"
    />
  </div>
</template>

<style scoped>
.skeleton {
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow: hidden;
  border-radius: 10px;
}
.skeleton-line {
  height: 14px;
  border-radius: 6px;
  background: linear-gradient(
    100deg,
    color-mix(in srgb, var(--gray-200) 80%, transparent) 20%,
    color-mix(in srgb, var(--gray-100) 90%, white 30%) 40%,
    color-mix(in srgb, var(--gray-200) 80%, transparent) 60%
  );
  background-size: 200% 100%;
  animation: skeleton-shimmer 1.4s ease-in-out infinite;
}
@keyframes skeleton-shimmer {
  0% { background-position: 120% 0; }
  100% { background-position: -40% 0; }
}
</style>