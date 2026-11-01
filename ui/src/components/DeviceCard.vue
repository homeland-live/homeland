<script setup lang="ts">
import { computed } from 'vue';
import { RouterLink } from 'vue-router';
import { VCard, VCardTitle, VCardSubtitle, VCardText } from 'vuetify/components/VCard';
import { VChip } from 'vuetify/components/VChip';
import { VTooltip } from 'vuetify/components/VTooltip';
import TimeAgoSync from '@/components/TimeAgoSync.vue';
import DataList from '@/components/DataList.vue';
import type { Z2MDevice } from '@/api';

const props = defineProps<{
  device: Z2MDevice;
}>();

const lastSeen = computed<string | undefined>(() => {
  const value = props.device.state?.last_seen;
  return typeof value === 'string' ? value : undefined;
});

const subtitle = computed<string>(() => {
  const spec = props.device.spec;
  if (spec.definition) {
    return [spec.definition.vendor, spec.definition.model].filter(Boolean).join(' · ');
  }
  return spec.model_id || spec.ieee_address;
});

// Indicator describes one status icon shown next to the device name.
interface Indicator {
  class: string;
  title: string;
}

// indicators derives the handler status icons from the compiled script the API
// reports for the device:
// - a compile error shows a single red warning triangle: neither the handler
//   nor its scheduled jobs run, so no other indicator applies;
// - an exported handle(ctx, state, prevState) function shows a lightning (green/filled
//   when the device has reported state, a plain outline otherwise);
// - exported jobs show a clock.
// A script may export both a handle function and jobs, so both indicators can
// appear together. A device with no script shows nothing.
const indicators = computed<Indicator[]>(() => {
  const handler = props.device.handler;
  if (!handler) {
    return [];
  }
  if (handler.error) {
    return [{ class: 'bi bi-exclamation-circle text-error', title: 'Script error' }];
  }

  const result: Indicator[] = [];
  if (handler.hasHandle) {
    result.push({ class: 'bi bi-lightning', title: 'Event handler enabled' });
  }
  if (handler.jobCount > 0) {
    result.push({
      class: 'bi bi-clock',
      title: `${handler.jobCount} scheduled job${handler.jobCount > 1 ? 's' : ''}`,
    });
  }
  return result;
});
</script>

<template>
  <VCard height="100%">
    <VCardTitle>
      <RouterLink
        :to="{ name: 'DeviceView', params: { id: props.device.spec.ieee_address } }"
        class="link-decoration-none"
        title="Open device details"
      >
        <i
          v-for="indicator in indicators"
          :key="indicator.class"
          class="me-1"
          :class="indicator.class"
          :title="indicator.title"
        />
        {{ props.device.spec.friendly_name }}
      </RouterLink>
    </VCardTitle>
    <VCardSubtitle>{{ subtitle }}</VCardSubtitle>
    <VCardText class="py-2">
      <div class="d-flex flex-wrap ga-2">
        <VChip size="small" label>{{ props.device.spec.type }}</VChip>
        <VChip v-if="props.device.spec.power_source" size="small" label>
          {{ props.device.spec.power_source }}
        </VChip>
        <VTooltip v-if="props.device.state" location="bottom">
          <template #activator="{ props: tooltipProps }">
            <VChip v-bind="tooltipProps" size="small" label color="success">reporting</VChip>
          </template>
          <DataList :data="props.device.state" />
        </VTooltip>
        <VChip v-else size="small" label>no state</VChip>
      </div>
      <div class="text-medium-emphasis mt-2 text-right">
        <span class="me-2">last seen:</span>
        <TimeAgoSync :value="lastSeen" />
      </div>
    </VCardText>
  </VCard>
</template>

<style scoped>
.state-tooltip {
  max-width: 320px;
}

.state-row {
  display: flex;
  gap: 1rem;
  justify-content: space-between;
}

.state-value {
  font-family: monospace;
  text-align: right;
  overflow-wrap: anywhere;
}
</style>
