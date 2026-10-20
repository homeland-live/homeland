<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { VCard, VCardTitle, VCardText } from 'vuetify/components/VCard';
import { VContainer, VRow, VCol } from 'vuetify/components/VGrid';
import { VChip } from 'vuetify/components/VChip';
import { VAlert } from 'vuetify/components/VAlert';
import { VProgressCircular } from 'vuetify/components/VProgressCircular';
import {
  getZ2MBridgeInfo,
  getZ2MBridgeHealth,
  type Z2MBridgeInfo,
  type Z2MBridgeHealth,
} from '@/api';

const info = ref<Z2MBridgeInfo | undefined>();
const health = ref<Z2MBridgeHealth | undefined>();
const infoError = ref<string | undefined>();
const healthError = ref<string | undefined>();
const loading = ref<boolean>(true);

function shortCommit(commit: string): string {
  return commit ? commit.slice(0, 7) : 'n/a';
}

function loadAverage(values: number[]): string {
  if (!values || !values.length) {
    return 'n/a';
  }
  return values.map((v) => v.toFixed(2)).join(' · ');
}

function uptime(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const parts: string[] = [];
  if (d) {
    parts.push(`${d}d`);
  }
  if (h || d) {
    parts.push(`${h}h`);
  }
  parts.push(`${m}m`);
  return parts.join(' ');
}

onMounted(() => {
  Promise.all([getZ2MBridgeInfo(), getZ2MBridgeHealth()])
    .then(([infoData, healthData]) => {
      if (infoData.error) {
        infoError.value = infoData.error.msg;
      } else {
        info.value = infoData.info;
      }
      if (healthData.error) {
        healthError.value = healthData.error.msg;
      } else {
        health.value = healthData.health;
      }
    })
    .finally(() => {
      loading.value = false;
    });
});
</script>

<template>
  <VContainer>
    <div class="text-headline-large text-center my-3">Bridge</div>

    <div v-if="loading" class="d-flex justify-center mt-8">
      <VProgressCircular indeterminate />
    </div>

    <VRow v-else justify="center" class="mt-1">
      <VCol cols="12" sm="6" lg="4">
        <VCard v-if="info">
          <VCardTitle class="text-medium-emphasis text-center">Info</VCardTitle>
          <VCardText class="text-body-large">
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">version</VCol>
              <VCol class="text-start ps-2">{{ info.version }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">commit</VCol>
              <VCol class="text-start ps-2">{{ shortCommit(info.commit) }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">coordinator</VCol>
              <VCol class="text-start ps-2">{{ info.coordinator.type }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">channel</VCol>
              <VCol class="text-start ps-2">{{ info.network.channel }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">permit join</VCol>
              <VCol class="text-start ps-2">
                <VChip size="small" label :color="info.permit_join ? 'warning' : undefined">
                  {{ info.permit_join ? 'open' : 'closed' }}
                </VChip>
              </VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">restart required</VCol>
              <VCol class="text-start ps-2">
                <VChip size="small" label :color="info.restart_required ? 'error' : 'success'">
                  {{ info.restart_required ? 'yes' : 'no' }}
                </VChip>
              </VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">mqtt server</VCol>
              <VCol class="text-start ps-2">{{ info.mqtt.server }}</VCol>
            </VRow>
          </VCardText>
        </VCard>
        <VAlert v-else-if="infoError" type="warning" variant="tonal">
          {{ infoError }}
        </VAlert>
      </VCol>

      <VCol cols="12" sm="6" lg="4">
        <VCard v-if="health">
          <VCardTitle class="text-medium-emphasis text-center">Health</VCardTitle>
          <VCardText class="text-body-large">
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">mqtt</VCol>
              <VCol class="text-start ps-2">
                <VChip size="small" label :color="health.mqtt.connected ? 'success' : 'error'">
                  {{ health.mqtt.connected ? 'connected' : 'disconnected' }}
                </VChip>
              </VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">uptime</VCol>
              <VCol class="text-start ps-2">{{ uptime(health.process.uptime_sec) }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">process memory</VCol>
              <VCol class="text-start ps-2">
                {{ health.process.memory_used_mb.toFixed(0) }} MB ({{
                  health.process.memory_percent.toFixed(1)
                }}%)
              </VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">os memory</VCol>
              <VCol class="text-start ps-2">
                {{ health.os.memory_used_mb.toFixed(0) }} MB ({{
                  health.os.memory_percent.toFixed(1)
                }}%)
              </VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">load average</VCol>
              <VCol class="text-start ps-2">{{ loadAverage(health.os.load_average) }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">mqtt published</VCol>
              <VCol class="text-start ps-2">{{ health.mqtt.published }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">mqtt received</VCol>
              <VCol class="text-start ps-2">{{ health.mqtt.received }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">devices</VCol>
              <VCol class="text-start ps-2">{{ Object.keys(health.devices).length }}</VCol>
            </VRow>
          </VCardText>
        </VCard>
        <VAlert v-else-if="healthError" type="warning" variant="tonal">
          {{ healthError }}
        </VAlert>
      </VCol>
    </VRow>
  </VContainer>
</template>
