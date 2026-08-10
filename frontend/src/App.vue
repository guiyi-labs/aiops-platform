<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import ConsoleLayout from './components/ConsoleLayout.vue'

const route = useRoute()
const consoleRoute = computed(() => !route.meta.public)
</script>

<template>
  <div class="route-viewport">
    <ConsoleLayout v-if="consoleRoute" shell>
      <RouterView />
    </ConsoleLayout>
    <RouterView v-else />
  </div>
</template>
