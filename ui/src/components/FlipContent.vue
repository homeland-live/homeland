<script setup lang="ts">
import { ref, computed, useSlots, useTemplateRef } from 'vue';

withDefaults(
  defineProps<{
    withIcon?: boolean;
  }>(),
  {
    withIcon: true,
  },
);

const slots = useSlots();

const flipElem = useTemplateRef('flipElem');

const isHeads = ref<boolean>(true);

// rotation applied to the element; CSS transitions animate between values.
const rotateY = ref<number>(0);

// true while the first half of the flip (0 -> 90) is running, so the
// transitionend handler knows to swap content and start the second half.
const flipping = ref<boolean>(false);

const hasTails = computed<boolean>(() => !!slots.tails);

function flip() {
  if (!flipElem.value || !hasTails.value || flipping.value) {
    return;
  }
  flipping.value = true;
  rotateY.value = 90; // first half: rotate edge-on (power2.in)
}

function onTransitionEnd() {
  if (flipping.value) {
    // reached 90deg: swap the visible slot, then flip back in from -90deg.
    isHeads.value = !isHeads.value;
    flipping.value = false;
    rotateY.value = -90;
    // next frame, animate -90 -> 0 (power2.out) via the "back" transition.
    requestAnimationFrame(() => {
      rotateY.value = 0;
    });
  }
}
</script>

<template>
  <span
    ref="flipElem"
    class="flip-container"
    :class="{ 'flip-out': flipping }"
    :style="{ transform: `rotateY(${rotateY}deg)` }"
    @click="flip"
    @transitionend="onTransitionEnd"
  >
    <span v-if="withIcon && hasTails" class="opacity-50 text-sm me-1">
      <i class="bi" :class="isHeads ? 'bi-front' : 'bi-back'" />
    </span>
    <slot v-if="isHeads" name="heads" />
    <slot v-else name="tails" />
  </span>
</template>

<style scoped>
.flip-container {
  display: inline-block;
  transform-style: preserve-3d;
  /* second half of the flip: -90 -> 0, ease-out */
  transition: transform 0.3s cubic-bezier(0, 0, 0.4, 1);
}

/* first half of the flip: 0 -> 90, ease-in */
.flip-container.flip-out {
  transition: transform 0.3s cubic-bezier(0.4, 0, 1, 1);
}
</style>
