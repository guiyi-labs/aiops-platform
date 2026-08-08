<template>
  <RouterView v-slot="{ Component }">
    <Transition name="page" mode="out-in" appear>
      <component :is="Component" />
    </Transition>
  </RouterView>
</template>