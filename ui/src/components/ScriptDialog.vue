<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { VAlert } from 'vuetify/components/VAlert';
import { VProgressCircular } from 'vuetify/components/VProgressCircular';
import BaseDialog from '@/components/vuetify/BaseDialog.vue';
import CodeViewer from '@/components/CodeViewer.vue';
import { getDeviceScript, type Script, type Err } from '@/api';

// `error` is the device handler's compile error, if any; the dialog shows it
// alongside the source. The source is either passed in directly (`script`,
// e.g. for an orphan script already loaded in a listing) or, when only
// `ieeeAddress` is given, fetched here keyed by the device's IEEE address.
const props = defineProps<{
  ieeeAddress?: string;
  script?: Script;
  error?: Err;
}>();

defineEmits<{
  (e: 'close'): void;
}>();

const script = ref<Script | undefined>(props.script);
const loading = ref<boolean>(false);
const loadError = ref<Err | undefined>();

// A failed load takes precedence: without the source there is nothing else to
// show, so the handler's compile error would only be noise.
const alertError = computed<Err | undefined>(() => loadError.value ?? props.error);

const title = computed<string>(() => script.value?.filename || props.ieeeAddress || 'Script');

onMounted(() => {
  // Source already supplied: nothing to fetch.
  if (props.script || !props.ieeeAddress) {
    loading.value = false;
    return;
  }
  getDeviceScript(props.ieeeAddress).then((data) => {
    loading.value = false;
    if (data.error) {
      loadError.value = data.error;
      return;
    }
    script.value = data.script ?? undefined;
  });
});
</script>

<template>
  <BaseDialog max-width="42rem" @close="$emit('close')">
    <template #header>
      {{ title }}
    </template>
    <template #body>
      <VAlert v-if="alertError" type="error" variant="tonal">
        {{ alertError.msg }}
      </VAlert>
      <div v-if="loading" class="d-flex justify-center my-6">
        <VProgressCircular indeterminate />
      </div>
      <CodeViewer v-else-if="script" :code="script.content" class="mt-3" />
      <VAlert v-else-if="!alertError" type="info" variant="tonal">
        This device has no script.
      </VAlert>
    </template>
  </BaseDialog>
</template>
