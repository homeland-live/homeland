<script setup lang="ts">
import { ref, computed } from 'vue';
import { VAlert } from 'vuetify/components/VAlert';
import { VBtn } from 'vuetify/components/VBtn';
import ScriptDialog from '@/components/ScriptDialog.vue';
import { type Handler, type Err } from '@/api';

const props = defineProps<{
  handler: Handler;
  ieeeAddress: string;
}>();

const error = computed<Err | undefined>(() => props.handler?.error);

const isScriptDialogVisible = ref<boolean>(false);

function showScriptDialog() {
  isScriptDialogVisible.value = true;
}

function hideScriptDialog() {
  isScriptDialogVisible.value = false;
}
</script>

<template>
  <VAlert :type="error ? 'error' : 'success'" variant="tonal" :class="$attrs.class">
    <div>
      <template v-if="error">
        <div class="text-title-medium">Script error</div>
        <div v-if="error" class="text-body-medium mt-1">
          {{ error.msg }}
        </div>
      </template>
      <template v-else>
        <div class="text-title-medium">Script enabled</div>
        <div v-if="handler.hasHandle" class="mt-1">
          <i class="bi bi-lightning me-1" />Event handler enabled
        </div>
        <div v-if="handler.jobCount > 0" class="mt-1">
          <i class="bi bi-clock me-1" />
          {{ handler.jobCount }} scheduled job{{ handler.jobCount > 1 ? 's' : '' }}
        </div>
      </template>
    </div>
    <div class="d-flex justify-end">
      <VBtn variant="text" size="small" @click="showScriptDialog">show script</VBtn>
    </div>
  </VAlert>
  <teleport to="body">
    <ScriptDialog
      v-if="isScriptDialogVisible"
      :ieee-address="ieeeAddress"
      :error="error"
      @close="hideScriptDialog"
    />
  </teleport>
</template>
