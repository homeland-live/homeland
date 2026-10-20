<script setup lang="ts">
import BaseDialog from '@/components/vuetify/BaseDialog.vue';

defineProps<{ data?: object }>();

defineEmits<{
  (e: 'close'): void;
}>();
</script>

<template>
  <BaseDialog max-width="42rem" @close="$emit('close')">
    <template #body>
      <pre class="data-json">{{ JSON.stringify(data, null, 2) }}</pre>
    </template>
  </BaseDialog>
</template>

<style scoped>
.data-json {
  margin: 0;
  font-family: monospace;
  font-size: 0.8125rem;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
