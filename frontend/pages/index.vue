<script setup lang="ts">
definePageMeta({ middleware: 'auth' })

onMounted(() => {
  navigateTo('/dashboard')
})
</script>

<template>
  <div />
</template>
