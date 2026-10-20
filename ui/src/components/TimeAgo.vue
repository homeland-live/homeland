<script setup lang="ts">
import { computed } from 'vue';
import { DateTime, type ToRelativeUnit, type StringUnitLength } from 'luxon';
import FlipContent from '@/components/FlipContent.vue';
import { isString, isNumber, isDate } from '@/util/type';
import { toRelative } from '@/util/datetime';

const props = defineProps<{
  value?: string | number | Date | DateTime;
  unit?: ToRelativeUnit;
  fmtstyle?: StringUnitLength;
}>();

const title = computed<string>(() => (!props.value ? 'undefined' : props.value.toString()));
const dt = computed<DateTime | undefined>(() => {
  if (isString(props.value)) {
    return DateTime.fromISO(props.value as string);
  }
  if (DateTime.isDateTime(props.value)) {
    return props.value;
  }
  if (isDate(props.value)) {
    return DateTime.fromJSDate(props.value as Date);
  }
  if (isNumber(props.value)) {
    return DateTime.fromSeconds(props.value as number);
  }
  return undefined;
});
const relative = computed<string>(() => {
  if (!dt.value || !dt.value.isValid) {
    return 'n/a';
  }
  return toRelative(dt.value, { unit: props.unit, style: props.fmtstyle });
});
const formatted = computed<string>(() => {
  if (!dt.value || !dt.value.isValid) {
    return 'n/a';
  }
  return dt.value.toFormat('yyyy-MM-dd HH:mm');
});
</script>

<template>
  <FlipContent :title="title" :with-icon="false">
    <template #heads>{{ relative }}</template>
    <template #tails>{{ formatted }}</template>
  </FlipContent>
</template>
