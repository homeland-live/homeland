<script setup lang="ts">
import { VSnackbar } from 'vuetify/components/VSnackbar';
import { VBtn } from 'vuetify/components/VBtn';
import { useToastStore } from '@/stores/toast';
import type { ToastType } from '@/types/toast';

const toasts = useToastStore();

// color maps a toast type to a Vuetify color. "default" has no color (uses the
// theme's surface), so it returns undefined.
function color(type?: ToastType): string | undefined {
  if (!type || type === 'default') {
    return undefined;
  }
  return type;
}
</script>

<template>
  <div class="toast-host">
    <VSnackbar
      v-for="toast in toasts.toasts"
      :key="toast.id"
      :model-value="true"
      :color="color(toast.type)"
      :timeout="toast.timeout"
      location="top end"
      class="toast-item"
      @update:model-value="toasts.dismiss(toast.id)"
    >
      <div class="font-weight-medium">{{ toast.title }}</div>
      <div v-if="toast.subtitle" class="text-caption">{{ toast.subtitle }}</div>
      <div v-if="toast.body" class="text-body-2">{{ toast.body }}</div>

      <template #actions>
        <VBtn
          icon="bi bi-x-lg"
          variant="text"
          size="small"
          aria-label="dismiss"
          @click="toasts.dismiss(toast.id)"
        />
      </template>
    </VSnackbar>
  </div>
</template>
