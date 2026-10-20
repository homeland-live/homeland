<script setup lang="ts">
import { ref, computed, useSlots, onMounted } from 'vue';
import { useDisplay } from 'vuetify';
import { VDialog } from 'vuetify/components/VDialog';
import { VCard, VCardText, VCardActions } from 'vuetify/components/VCard';
import { VToolbar, VToolbarTitle } from 'vuetify/components/VToolbar';
import { VBtn } from 'vuetify/components/VBtn';

const props = withDefaults(
  defineProps<{
    scrim?: boolean;
    persistent?: boolean;
    closable?: boolean;
    minWidth?: string;
    maxWidth?: string;
    headerClass?: string;
    bodyClass?: string;
    footerClass?: string;
  }>(),
  {
    scrim: true,
    persistent: true,
    closable: true,
    minWidth: 'sm',
    maxWidth: 'none',
    headerClass: undefined,
    bodyClass: undefined,
    footerClass: undefined,
  },
);

const emit = defineEmits<{
  (e: 'close'): void;
}>();

const slots = useSlots();
const { mobile } = useDisplay();

const visible = ref<boolean>(false);

const minWidth = computed<string | undefined>(() =>
  !mobile.value ? normalizeWidth(props.minWidth) : undefined,
);
const maxWidth = computed<string | undefined>(() =>
  !mobile.value ? normalizeWidth(props.maxWidth) : undefined,
);

const showHeader = computed<boolean>(() => !!slots.header || props.closable);
const showFooter = computed<boolean>(() => !!slots.footer);

onMounted(() => {
  visible.value = true;
});

function close() {
  visible.value = false;
}

function onAfterLeave() {
  emit('close');
}

function normalizeWidth(width: string): string {
  switch (width) {
    case 'xs':
      return '300px';
    case 'sm':
      return '500px';
    case 'lg':
      return '800px';
    case 'xl':
      return '1140px';
    default:
      return width;
  }
}

defineExpose({ close });
</script>

<template>
  <VDialog
    v-model="visible"
    :scrim="scrim"
    :persistent="persistent"
    :min-width="minWidth"
    :max-width="maxWidth"
    width="fit-content"
    :fullscreen="mobile"
    @keydown.esc="close()"
    @after-leave="onAfterLeave"
  >
    <VCard>
      <VToolbar
        v-if="showHeader"
        density="compact"
        color="surface"
        class="px-1 mt-1"
        :class="headerClass"
      >
        <VToolbarTitle>
          <slot name="header" />
        </VToolbarTitle>
        <template #append>
          <slot name="toolbar" />
          <VBtn v-if="closable" icon="bi bi-x-lg" variant="text" size="small" @click="close" />
        </template>
      </VToolbar>
      <VCardText :class="bodyClass">
        <slot name="body" />
      </VCardText>
      <VCardActions v-if="showFooter" class="px-4 mb-2" :class="footerClass">
        <slot name="footer" />
      </VCardActions>
    </VCard>
  </VDialog>
</template>
