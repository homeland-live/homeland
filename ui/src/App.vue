<script setup lang="ts">
import { ref, watch } from 'vue';
import { useDisplay } from 'vuetify';
import { VApp } from 'vuetify/components/VApp';
import { VNavigationDrawer } from 'vuetify/components/VNavigationDrawer';
import { VList, VListItem } from 'vuetify/components/VList';
import { VDivider } from 'vuetify/components/VDivider';
import { VBtn } from 'vuetify/components/VBtn';
import { VMain } from 'vuetify/components/VMain';
import ToastHost from '@/components/ToastHost.vue';

const { mobile } = useDisplay();

// Drawer is open by default on desktop, closed on mobile. Keep it in sync when
// the viewport crosses the mobile breakpoint.
const drawer = ref(!mobile.value);
watch(mobile, (isMobile) => {
  drawer.value = !isMobile;
});

function closeOnMobile() {
  if (mobile.value) {
    drawer.value = false;
  }
}
</script>

<template>
  <VApp>
    <VBtn
      v-if="mobile"
      icon="true"
      variant="text"
      class="app-nav-toggle"
      aria-label="Toggle navigation"
      @click="drawer = !drawer"
    >
      <i class="bi bi-list" />
    </VBtn>
    <VNavigationDrawer v-model="drawer" :permanent="!mobile" :temporary="mobile">
      <div class="text-headline-small d-flex align-center px-4 py-3">
        <i class="bi bi-house-gear me-2" />
        <span>Homeland</span>
      </div>
      <VDivider />
      <VList nav>
        <VListItem title="Bridge" :to="{ name: 'BridgeView' }" link @click="closeOnMobile" />
        <VListItem title="Devices" :to="{ name: 'DevicesView' }" link @click="closeOnMobile" />
        <VDivider />
        <VListItem title="Scripts" :to="{ name: 'ScriptsView' }" link @click="closeOnMobile" />
        <VDivider />
        <VListItem
          title="BuildInfo"
          :to="{ name: 'StatusBuildView' }"
          link
          @click="closeOnMobile"
        />
      </VList>
    </VNavigationDrawer>
    <VMain>
      <router-view />
    </VMain>
    <ToastHost />
  </VApp>
</template>

<style scoped>
.app-nav-toggle {
  position: fixed;
  top: 8px;
  left: 8px;
  z-index: 2000;
}
</style>
