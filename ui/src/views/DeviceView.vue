<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useRoute } from 'vue-router';
import { useClipboard } from '@vueuse/core';
import { useToastStore } from '@/stores/toast';
import { VContainer, VRow, VCol } from 'vuetify/components/VGrid';
import { VCard, VCardTitle, VCardSubtitle, VCardText } from 'vuetify/components/VCard';
import { VChip } from 'vuetify/components/VChip';
import { VBtn } from 'vuetify/components/VBtn';
import { VAlert } from 'vuetify/components/VAlert';
import { VTable } from 'vuetify/components/VTable';
import { VTooltip } from 'vuetify/components/VTooltip';
import { VProgressCircular } from 'vuetify/components/VProgressCircular';
import { VTabs, VTab, VTabsWindow, VTabsWindowItem } from 'vuetify/components/VTabs';
import BasePaginator from '@/components/vuetify/BasePaginator.vue';
import DataDialog from '@/components/DataDialog.vue';
import TimeAgo from '@/components/TimeAgo.vue';
import DataList from '@/components/DataList.vue';
import HandlerAlert from '@/components/HandlerAlert.vue';
import {
  listDeviceEvents,
  type Z2MDeviceSpec,
  type Z2MStateSchema,
  type DeviceState,
  type Event,
  type Page,
  type Err,
  type Handler,
} from '@/api';

const defaultPageNumber = 1;
const defaultPageSize = 10;

const route = useRoute();
const ieeeAddress = computed<string>(() => String(route.params.id));

const spec = ref<Z2MDeviceSpec | null>(null);
const schema = ref<Z2MStateSchema | null>(null);
const state = ref<DeviceState | null>(null);
const handler = ref<Handler | null>(null);
const events = ref<Event[]>([]);

// nonScalarExposeTypes are the expose types whose values are objects or arrays
// (composite features, list items) rather than a single scalar. They don't fit
// a single table cell, so they are excluded from the event table columns.
const nonScalarExposeTypes = new Set(['composite', 'list']);

// primaryProps are the names of the device's regular-usage state properties
// that render as a single value (the keys of the state schema's primaryProps
// map, minus composite/list props). They become the event table's value
// columns. Empty when the device has no definition/schema, in which case the
// table shows only ReceivedAt.
const primaryProps = computed<string[]>(() =>
  Object.entries(schema.value?.primaryProps ?? {})
    .filter(([, type]) => !nonScalarExposeTypes.has(type))
    .map(([prop]) => prop),
);
const page = ref<Page>({ num: defaultPageNumber, size: defaultPageSize, count: 0 });
const loading = ref<boolean>(true);
const loaded = ref<boolean>(false);
const error = ref<Err | undefined>();

const isDataDialogVisible = ref<boolean>(false);
const dataToShow = ref<object | undefined>();

// detailTab selects which detail panel (current state vs exposes) is shown.
const detailTab = ref<'state' | 'exposes'>('state');

const subtitle = computed<string>(() => {
  if (!spec.value) {
    return '';
  }
  if (spec.value.definition) {
    return [spec.value.definition.vendor, spec.value.definition.model].filter(Boolean).join(' · ');
  }
  return spec.value.model_id || spec.value.ieee_address;
});

// exposes flattens the device definition's exposes to leaf properties (a
// composite expose like "switch" has no property of its own, but its
// sub-features do), keyed by property name so the detail page can render them
// with DataList. Each value is the formatted "<type> (<unit>)" string, with
// the unit suffix dropped when the expose has no unit.
const exposes = computed<Record<string, string>>(() => {
  const defs = spec.value?.definition?.exposes;
  if (!defs) {
    return {};
  }
  const leaves = defs.flatMap((expose) => (expose.property ? [expose] : (expose.features ?? [])));
  const out: Record<string, string> = {};
  for (const expose of leaves) {
    out[expose.property] = expose.unit ? `${expose.type} (${expose.unit})` : expose.type;
  }
  return out;
});

onMounted(() => {
  load(defaultPageNumber);
});

// The events endpoint is keyed by IEEE address and returns the device's
// metadata, state, and a page of events in one response.
function load(pageNum: number): Promise<void> {
  loading.value = true;
  error.value = undefined;
  return listDeviceEvents(ieeeAddress.value, {
    pagenum: pageNum,
    pagesize: page.value.size,
  }).then((data) => {
    loading.value = false;
    loaded.value = true;
    if (data.error) {
      error.value = data.error;
      return;
    }
    spec.value = data.spec;
    schema.value = data.schema;
    state.value = data.state;
    handler.value = data.handler;
    events.value = data.events || [];
    if (data.page) {
      page.value = data.page;
    }
  });
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined) {
    return 'null';
  }
  if (typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
}

function payloadJSON(payload: Record<string, unknown>): string {
  return JSON.stringify(payload, null, 2);
}

// cellValue formats a single primary-prop value from an event's payload. A
// prop absent from the payload (partial state update) renders as an em dash.
function cellValue(payload: Record<string, unknown>, prop: string): string {
  if (!(prop in payload)) {
    return '—';
  }
  return formatValue(payload[prop]);
}

const toasts = useToastStore();
const { copy, isSupported: clipboardSupported } = useClipboard();
// copiedId tracks the row whose payload was just copied, so its button can
// briefly flip to a check icon as feedback (alongside the toast).
const copiedId = ref<number | null>(null);
let copiedTimer: ReturnType<typeof setTimeout> | undefined;

function copyEvent(event: Event): void {
  copy(payloadJSON(event.payload)).then(() => {
    copiedId.value = event.id;
    clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => {
      copiedId.value = null;
    }, 1500);
    toasts.success('Copied', 'Event payload copied to clipboard.');
  });
}

function showDataDialog(data: object) {
  dataToShow.value = data;
  isDataDialogVisible.value = true;
}

function hideDataDialog() {
  isDataDialogVisible.value = false;
}
</script>

<template>
  <VContainer>
    <div class="d-flex align-center my-3">
      <VBtn
        icon="bi bi-chevron-left"
        variant="text"
        size="small"
        :to="{ name: 'DevicesView' }"
        class="me-2"
      />
      <span class="text-headline-large">{{ spec ? spec.friendly_name : ieeeAddress }}</span>
    </div>

    <div v-if="!loaded && loading" class="d-flex justify-center mt-8">
      <VProgressCircular indeterminate />
    </div>

    <VAlert v-else-if="error" type="error" variant="tonal" class="mt-4">
      {{ error.msg }}
    </VAlert>

    <VRow v-else class="mt-1">
      <!-- Device details -->
      <VCol cols="12" md="5" lg="4">
        <VCard height="100%">
          <VCardTitle>{{ spec ? spec.ieee_address : ieeeAddress }}</VCardTitle>
          <VCardSubtitle>{{ subtitle }}</VCardSubtitle>
          <VCardText class="text-body-large">
            <VAlert v-if="!spec" type="info" variant="tonal" density="compact">
              Device is not currently known to the bridge; showing stored events only.
            </VAlert>
            <div v-else class="d-flex flex-wrap align-center ga-2 mb-3">
              <VChip size="small" label>{{ spec.type }}</VChip>
              <VChip v-if="spec.power_source" size="small" label>
                {{ spec.power_source }}
              </VChip>
              <VBtn density="comfortable" size="small" @click="showDataDialog(spec)">
                show full spec
              </VBtn>
            </div>

            <VTabs v-model="detailTab" density="compact" align-tabs="center" class="mt-2">
              <VTab value="state">state</VTab>
              <VTab value="exposes">exposes</VTab>
            </VTabs>
            <VTabsWindow v-model="detailTab" class="mt-3">
              <VTabsWindowItem value="state" class="tab-item-centered">
                <DataList v-if="state?.attrs" :data="state.attrs" />
                <VAlert
                  v-else
                  type="warning"
                  variant="tonal"
                  density="compact"
                  class="text-body-medium"
                >
                  No data.
                </VAlert>
              </VTabsWindowItem>
              <VTabsWindowItem value="exposes" class="tab-item-centered">
                <DataList v-if="Object.keys(exposes).length" :data="exposes" sort />
                <VAlert
                  v-else
                  type="warning"
                  variant="tonal"
                  density="compact"
                  class="text-body-medium"
                >
                  No data.
                </VAlert>
              </VTabsWindowItem>
            </VTabsWindow>
            <HandlerAlert
              v-if="handler"
              :handler="handler"
              :ieee-address="ieeeAddress"
              class="mt-3"
            />
          </VCardText>
        </VCard>
      </VCol>

      <!-- Event history -->
      <VCol cols="12" md="7" lg="8">
        <VCard height="100%">
          <VCardTitle class="d-flex align-center justify-space-between">
            <div>
              events <span class="text-medium-emphasis">({{ page.count }})</span>
            </div>
            <BasePaginator :page="page" @change="load" />
          </VCardTitle>
          <VCardText>
            <div v-if="loading" class="d-flex justify-center my-6">
              <VProgressCircular indeterminate />
            </div>

            <VAlert v-else-if="!events.length" type="info" variant="tonal">
              No events recorded for this device yet.
            </VAlert>

            <VTable v-else density="compact" class="events-table">
              <thead>
                <tr>
                  <th
                    v-for="prop in primaryProps"
                    :key="prop"
                    class="text-start font-weight-semibold"
                  >
                    {{ prop }}
                  </th>
                  <th class="text-start font-weight-semibold">received</th>
                  <th class="text-end" aria-label="copy"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="event in events" :key="event.id">
                  <td v-for="prop in primaryProps" :key="prop" class="event-cell">
                    {{ cellValue(event.payload, prop) }}
                  </td>
                  <td class="text-medium-emphasis">
                    <TimeAgo :value="event.createdAt" />
                  </td>
                  <td class="text-end">
                    <VTooltip location="start" max-width="400">
                      <template #activator="{ props: tooltipProps }">
                        <VBtn
                          v-bind="tooltipProps"
                          :icon="
                            copiedId === event.id ? 'bi bi-clipboard-check' : 'bi bi-clipboard'
                          "
                          :color="copiedId === event.id ? 'success' : undefined"
                          :disabled="!clipboardSupported"
                          variant="text"
                          size="small"
                          :aria-label="copiedId === event.id ? 'copied' : 'copy payload JSON'"
                          @click="copyEvent(event)"
                        />
                      </template>
                      <DataList :data="event.payload" />
                    </VTooltip>
                  </td>
                </tr>
              </tbody>
            </VTable>
          </VCardText>
        </VCard>
      </VCol>
    </VRow>
  </VContainer>
  <teleport to="body">
    <DataDialog v-if="isDataDialogVisible" :data="dataToShow" @close="hideDataDialog" />
  </teleport>
</template>

<style scoped>
.tab-item-centered {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.state-row {
  overflow-wrap: anywhere;
}

.state-value {
  font-family: monospace;
}

.events-table {
  overflow-wrap: anywhere;
}

.event-cell {
  font-family: monospace;
}
</style>
