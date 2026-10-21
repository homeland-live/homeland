<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { VContainer, VRow, VCol } from 'vuetify/components/VGrid';
import { VCard, VCardTitle, VCardText } from 'vuetify/components/VCard';
import { VChip } from 'vuetify/components/VChip';
import { VAlert } from 'vuetify/components/VAlert';
import { VProgressCircular } from 'vuetify/components/VProgressCircular';
import { VTreeview } from 'vuetify/components/VTreeview';
import { getHealth, type HealthCheck, type Err } from '@/api';

// HealthNode is a health check shaped for VTreeview. `id` is the node's path
// from the root, since check names repeat across branches. Leaves have no
// `children`, so the tree renders no toggle for them. An unhealthy leaf shows
// its error as the item's subtitle (`props` is passed to the list item); a
// parent's error only names its failing children, which the tree already shows.
interface HealthNode {
  id: string;
  title: string;
  isHealthy: boolean;
  props?: { subtitle: string };
  children?: HealthNode[];
}

const health = ref<HealthCheck | undefined>();
const loading = ref<boolean>(false);
const error = ref<Err | undefined>();

const items = computed<HealthNode[]>(() => (health.value ? [toNode(health.value)] : []));

onMounted(loadHealth);

function toNode(hc: HealthCheck, parent = ''): HealthNode {
  const id = parent ? `${parent}/${hc.name}` : hc.name;
  const children = hc.checks?.length ? hc.checks.map((c) => toNode(c, id)) : undefined;
  return {
    id,
    title: hc.name,
    isHealthy: hc.isHealthy,
    props: !children && hc.error ? { subtitle: hc.error } : undefined,
    children,
  };
}

function asNode(item: unknown): HealthNode {
  return item as HealthNode;
}

function loadHealth() {
  loading.value = true;
  getHealth()
    .then((resp) => {
      error.value = resp.error;
      health.value = resp.health;
    })
    .finally(() => {
      loading.value = false;
    });
}
</script>

<template>
  <VContainer>
    <div class="text-headline-large text-center my-3">Health</div>

    <div v-if="loading" class="d-flex justify-center mt-8">
      <VProgressCircular indeterminate />
    </div>

    <VAlert v-else-if="error" type="error" variant="tonal" class="mt-4">
      {{ error.msg }}
    </VAlert>

    <VRow v-else-if="health" justify="center" class="mt-1">
      <VCol cols="12" md="8" lg="6">
        <VCard>
          <VCardTitle class="d-flex align-center justify-center ga-2 text-medium-emphasis">
            <span>Status</span>
            <VChip size="small" label :color="health.isHealthy ? 'success' : 'error'">
              {{ health.isHealthy ? 'healthy' : 'unhealthy' }}
            </VChip>
          </VCardTitle>
          <VCardText>
            <VTreeview
              :items="items"
              item-value="id"
              item-title="title"
              density="compact"
              expand-icon="bi bi-chevron-right"
              collapse-icon="bi bi-chevron-down"
              open-all
            >
              <template #prepend="{ item }">
                <i
                  class="bi me-2"
                  :class="
                    asNode(item).isHealthy
                      ? 'bi-check-circle-fill text-success'
                      : 'bi-x-circle-fill text-error'
                  "
                  :aria-label="asNode(item).isHealthy ? 'healthy' : 'unhealthy'"
                  role="img"
                />
              </template>
            </VTreeview>
          </VCardText>
        </VCard>
      </VCol>
    </VRow>
  </VContainer>
</template>
