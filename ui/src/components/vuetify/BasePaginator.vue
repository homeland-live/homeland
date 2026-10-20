<script setup lang="ts">
import { computed } from 'vue';
import { VPagination } from 'vuetify/components/VPagination';
import { type Page } from '@/api';

const props = defineProps<{
  page: Page;
}>();

const emit = defineEmits<{
  (e: 'change', pagenum: number): void;
}>();

const current = computed<number>(() => props.page?.num ?? 0);

const total = computed<number>(() => {
  if (!props.page || !props.page.size) {
    return 0;
  }
  return Math.ceil(props.page.count / props.page.size);
});

function onPageChange(pagenum: number) {
  emit('change', pagenum);
}
</script>

<template>
  <VPagination
    :model-value="current"
    :length="total"
    :total-visible="4"
    prev-icon="bi bi-chevron-left"
    next-icon="bi bi-chevron-right"
    size="small"
    density="comfortable"
    @update:model-value="onPageChange"
  />
</template>
