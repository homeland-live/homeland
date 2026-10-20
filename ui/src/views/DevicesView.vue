<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { VContainer, VRow, VCol } from 'vuetify/components/VGrid';
import { VAlert } from 'vuetify/components/VAlert';
import { VProgressCircular } from 'vuetify/components/VProgressCircular';
import DeviceCard from '@/components/DeviceCard.vue';
import { getZ2MBridgeDevices, type Z2MDevice, type Err } from '@/api';

const devices = ref<Z2MDevice[]>([]);
const loading = ref<boolean>(true);
const error = ref<Err | undefined>();

// devices worth showing: the coordinator has no useful state, so it is
// filtered out (mirrors server-side device registry).
const filteredDevices = computed<Z2MDevice[]>(() =>
  devices.value
    .filter((s) => s.spec.type !== 'Coordinator')
    .sort((a, b) => a.spec.friendly_name.localeCompare(b.spec.friendly_name)),
);

onMounted(() => {
  getZ2MBridgeDevices()
    .then((resp) => {
      error.value = resp.error;
      devices.value = resp.devices || [];
    })
    .finally(() => {
      loading.value = false;
    });
});
</script>

<template>
  <VContainer>
    <div class="text-headline-large text-center my-3">Devices</div>

    <div v-if="loading" class="d-flex justify-center mt-8">
      <VProgressCircular indeterminate />
    </div>

    <VAlert v-else-if="error" type="error" variant="tonal" class="mt-4">
      {{ error.msg }}
    </VAlert>

    <VAlert v-else-if="!filteredDevices.length" type="info" variant="tonal" class="mt-4">
      No devices found.
    </VAlert>

    <VRow v-else class="mt-1">
      <VCol
        v-for="dev in filteredDevices"
        :key="dev.spec.ieee_address"
        cols="12"
        sm="6"
        md="4"
        lg="3"
      >
        <DeviceCard :device="dev" />
      </VCol>
    </VRow>
  </VContainer>
</template>
