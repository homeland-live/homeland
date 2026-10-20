<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { RouterLink } from 'vue-router';
import { VContainer } from 'vuetify/components/VGrid';
import { VAlert } from 'vuetify/components/VAlert';
import { VProgressCircular } from 'vuetify/components/VProgressCircular';
import { VTable } from 'vuetify/components/VTable';
import { VTooltip } from 'vuetify/components/VTooltip';
import ScriptDialog from '@/components/ScriptDialog.vue';
import { listScripts, type CompiledScript, type Script, type Err } from '@/api';

const scripts = ref<CompiledScript[]>([]);
const loading = ref<boolean>(true);
const error = ref<Err | undefined>();

const dialogScript = ref<Script | undefined>();
const dialogError = ref<Err | undefined>();
const dialogDeviceId = ref<string | undefined>();
const isDialogVisible = ref<boolean>(false);

function showScript(script: CompiledScript) {
  dialogScript.value = script.script ?? undefined;
  dialogError.value = script.handler?.error;
  dialogDeviceId.value = script.deviceId;
  isDialogVisible.value = true;
}

function hideScript() {
  isDialogVisible.value = false;
}

onMounted(() => {
  listScripts()
    .then((resp) => {
      error.value = resp.error;
      scripts.value = resp.scripts || [];
    })
    .finally(() => {
      loading.value = false;
    });
});
</script>

<template>
  <VContainer>
    <div class="text-headline-large text-center my-3">Scripts</div>

    <div v-if="loading" class="d-flex justify-center mt-8">
      <VProgressCircular indeterminate />
    </div>

    <VAlert v-else-if="error" type="error" variant="tonal" class="mt-4">
      {{ error.msg }}
    </VAlert>

    <VAlert v-else-if="!scripts.length" type="info" variant="tonal" class="mt-4">
      No scripts loaded.
    </VAlert>

    <template v-else>
      <VTable density="compact" hover class="scripts-table mt-4">
        <thead>
          <tr>
            <th class="text-center font-weight-semibold">Status</th>
            <th class="text-start font-weight-semibold">File</th>
            <th class="text-start font-weight-semibold">Device</th>
            <th class="text-center font-weight-semibold">Handler</th>
            <th class="text-center font-weight-semibold">Scheduled Jobs</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="script in scripts" :key="script.deviceId">
            <td class="text-center">
              <VTooltip v-if="script.handler?.error" location="top" max-width="400">
                <template #activator="{ props: tooltipProps }">
                  <i v-bind="tooltipProps" class="bi bi-exclamation-circle-fill text-error" />
                </template>
                {{ script.handler.error.msg }}
              </VTooltip>
              <VTooltip v-else-if="script.orphan" location="top" max-width="400">
                <template #activator="{ props: tooltipProps }">
                  <i v-bind="tooltipProps" class="bi bi-question-circle-fill text-error" />
                </template>
                No matching device: this script's file name does not match any known device, so it
                never runs.
              </VTooltip>
              <i v-else class="bi bi-check-circle-fill text-success" />
            </td>
            <td>
              <a class="script-link" @click="showScript(script)">
                <i class="bi bi-filetype-js me-1" />
                {{ script.script?.filename }}
              </a>
            </td>
            <td>
              <div class="text-medium-emphasis">
                <span v-if="script.orphan">{{ script.deviceId }}</span>
                <RouterLink
                  v-else
                  :to="{ name: 'DeviceView', params: { id: script.deviceId } }"
                  class="link-decoration-none text-medium-emphasis"
                >
                  {{ script.deviceId }}
                </RouterLink>
              </div>
            </td>
            <td class="text-center">
              <i v-if="script.handler?.hasHandle" class="bi bi-check-lg" />
              <span v-else class="text-medium-emphasis">−</span>
            </td>
            <td class="text-center">
              <template v-if="script.handler && script.handler.jobCount > 0">
                {{ script.handler.jobCount }}
              </template>
              <span v-else class="text-medium-emphasis">−</span>
            </td>
          </tr>
        </tbody>
      </VTable>
    </template>

    <teleport to="body">
      <ScriptDialog
        v-if="isDialogVisible"
        :script="dialogScript"
        :ieee-address="dialogDeviceId"
        :error="dialogError"
        @close="hideScript"
      />
    </teleport>
  </VContainer>
</template>

<style scoped>
.script-link {
  cursor: pointer;
}
</style>
