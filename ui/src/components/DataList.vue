<script setup lang="ts">
import { computed } from 'vue';
import { VChip } from 'vuetify/components/VChip';
import TimeAgoSync from '@/components/TimeAgoSync.vue';

defineOptions({
  name: 'DataList', // required to enable self-referencing
});

const props = withDefaults(
  defineProps<{
    data: Record<string, unknown>;
    // sort renders the properties ordered by key. Off by default, in which
    // case properties keep their insertion order. Propagates to nested lists;
    // arrays always keep their index order.
    sort?: boolean;
  }>(),
  {
    sort: false,
  },
);

// entries are the [key, value] pairs to render, optionally sorted by key.
const entries = computed<[string, unknown][]>(() => {
  const pairs = Object.entries(props.data);
  return props.sort && !Array.isArray(props.data)
    ? pairs.sort(([a], [b]) => a.localeCompare(b))
    : pairs;
});

function isObject(arg: unknown): arg is Record<string, unknown> {
  return arg !== null && typeof arg === 'object';
}

// formatValue renders a leaf value: an empty array or object as its JSON
// ("[]", "{}"), null as "null", anything else as its string.
function formatValue(value: unknown): string {
  return isObject(value) || value === null ? JSON.stringify(value) : String(value);
}
</script>

<template>
  <ul class="data-list">
    <li v-for="[key, value] in entries" :key="key">
      <span>{{ key }}:</span>
      <DataList v-if="isObject(value) && Object.keys(value).length" :data="value" :sort="sort" />
      <template v-else>
        <span class="ms-1" />
        <VChip label density="compact" size="small">
          <TimeAgoSync v-if="key === 'last_seen'" :value="String(value)" />
          <template v-else>{{ formatValue(value) }}</template>
        </VChip>
      </template>
    </li>
  </ul>
</template>

<style scoped>
.data-list {
  padding-inline-start: 1rem;
  list-style-type: '• ';
}

.data-list > li::marker {
  word-spacing: 1px;
}
</style>
