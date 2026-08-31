<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import ConsoleLayout from './components/ConsoleLayout.vue'

const route = useRoute()
// Public routes (e.g. /login) render without the console shell.
const consoleRoute = computed(() => !route.meta.public)
</script>

<template>
  <div class="route-viewport">
    <!-- ConsoleLayout resolves authenticated pages directly from the matched
         route, while public pages continue using the root RouterView. -->
    <ConsoleLayout v-if="consoleRoute" shell />
    <RouterView v-else />
  </div>
</template>
