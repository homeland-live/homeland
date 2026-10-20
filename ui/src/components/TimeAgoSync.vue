<script setup lang="ts">
import { computed } from 'vue';
import { DateTime } from 'luxon';
import {
  formatDate,
  type UseTimeAgoMessages,
  type UseTimeAgoUnit,
  type UseTimeAgoUnitNamesDefault,
} from '@vueuse/core';
import { UseTimeAgo } from '@vueuse/components';
import FlipContent from '@/components/FlipContent.vue';
import { isString, isDate } from '@/util/type';

const props = withDefaults(
  defineProps<{
    value?: string | Date | DateTime;
    fmt?: '' | 'short';
    unit?: UseTimeAgoUnitNamesDefault;
  }>(),
  {
    value: undefined,
    fmt: '',
    unit: undefined,
  },
);

// from https://github.com/vueuse/vueuse/blob/main/packages/core/useTimeAgo/index.ts
const DEFAULT_UNITS: UseTimeAgoUnit<UseTimeAgoUnitNamesDefault>[] = [
  { max: 60000, value: 1000, name: 'second' },
  { max: 2760000, value: 60000, name: 'minute' },
  { max: 72000000, value: 3600000, name: 'hour' },
  { max: 518400000, value: 86400000, name: 'day' },
  { max: 2419200000, value: 604800000, name: 'week' },
  { max: 28512000000, value: 2592000000, name: 'month' },
  { max: Number.POSITIVE_INFINITY, value: 31536000000, name: 'year' },
];

// from https://github.com/vueuse/vueuse/blob/main/packages/core/useTimeAgo/index.ts
const DEFAULT_MESSAGES: UseTimeAgoMessages<UseTimeAgoUnitNamesDefault> = {
  justNow: 'just now',
  past: (n) => (n.match(/\d/) ? `${n} ago` : n),
  future: (n) => (n.match(/\d/) ? `in ${n}` : n),
  month: (n, past) =>
    n === 1 ? (past ? 'last month' : 'next month') : `${n} month${n > 1 ? 's' : ''}`,
  year: (n, past) =>
    n === 1 ? (past ? 'last year' : 'next year') : `${n} year${n > 1 ? 's' : ''}`,
  day: (n, past) => (n === 1 ? (past ? 'yesterday' : 'tomorrow') : `${n} day${n > 1 ? 's' : ''}`),
  week: (n, past) =>
    n === 1 ? (past ? 'last week' : 'next week') : `${n} week${n > 1 ? 's' : ''}`,
  hour: (n) => `${n} hour${n > 1 ? 's' : ''}`,
  minute: (n) => `${n} minute${n > 1 ? 's' : ''}`,
  second: (n) => `${n} second${n > 1 ? 's' : ''}`,
  invalid: '',
};

const title = computed<string>(() => (!props.value ? 'undefined' : props.value.toString()));
const date = computed<Date | undefined>(() => {
  if (DateTime.isDateTime(props.value)) {
    return props.value.toJSDate();
  }
  if (isString(props.value)) {
    const dt = DateTime.fromISO(props.value as string);
    if (!dt.isValid) {
      return undefined;
    }
    return dt.toJSDate();
  }
  if (isDate(props.value)) {
    return props.value as Date;
  }
  return undefined;
});
const formatted = computed<string>(() =>
  !date.value ? '' : formatDate(date.value, 'YYYY-MM-DD HH:mm'),
);
const messages = computed<UseTimeAgoMessages<UseTimeAgoUnitNamesDefault> | undefined>(() =>
  props.fmt === 'short'
    ? {
        ...DEFAULT_MESSAGES,
        minute: (n: number) => `${n} min.`,
        second: (n: number) => `${n} sec.`,
      }
    : undefined,
);
const units = computed<UseTimeAgoUnit<UseTimeAgoUnitNamesDefault>[]>(() => {
  if (!props.unit || props.unit === 'year') {
    return DEFAULT_UNITS as UseTimeAgoUnit<UseTimeAgoUnitNamesDefault>[];
  }
  const idx = DEFAULT_UNITS.findIndex((u) => u.name === props.unit);
  if (idx === -1) {
    return DEFAULT_UNITS as UseTimeAgoUnit<UseTimeAgoUnitNamesDefault>[];
  }
  const scoped = DEFAULT_UNITS.slice(0, idx + 1);
  scoped[scoped.length - 1]!.max = Number.POSITIVE_INFINITY;
  return scoped as UseTimeAgoUnit<UseTimeAgoUnitNamesDefault>[];
});
</script>

<template>
  <FlipContent :title="title" :with-icon="false">
    <template #heads>
      <UseTimeAgo v-if="date" v-slot="{ timeAgo }" :time="date" :messages="messages" :units="units">
        {{ timeAgo }}
      </UseTimeAgo>
      <template v-else>n/a</template>
    </template>
    <template v-if="formatted" #tails>{{ formatted }}</template>
  </FlipContent>
</template>
