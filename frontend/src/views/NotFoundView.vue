<script setup lang="ts">
import ConsoleLayout from '../components/ConsoleLayout.vue'
</script>
<template>
  <ConsoleLayout title="页面不存在">
    <section class="panel"><h2>页面不存在</h2><p>请从左侧导航进入系统功能。</p><RouterLink to="/">返回系统概览</RouterLink></section>
  </ConsoleLayout>
</template>
