<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { VCard, VCardTitle, VCardText } from 'vuetify/components/VCard';
import { VContainer, VRow, VCol } from 'vuetify/components/VGrid';
import { getStatusBuildInfo, type BuildInfo, getDeps, type Dep } from '@/api';
import TimeAgo from '@/components/TimeAgo.vue';

const buildInfo = ref<BuildInfo | undefined>();
const uiDeps = ref<Dep[]>([]);

onMounted(() => {
  Promise.all([getStatusBuildInfo(), getDeps('ui')]).then(([buildInfoData, uiDepsData]) => {
    buildInfo.value = buildInfoData;
    uiDeps.value = uiDepsData;
  });
});
</script>

<template>
  <VContainer>
    <div class="text-headline-large text-center my-3">Build</div>
    <VRow justify="center" class="mt-1">
      <VCol cols="12" sm="6" lg="4">
        <VCard v-if="buildInfo">
          <VCardTitle class="text-medium-emphasis text-center">Server</VCardTitle>
          <VCardText class="text-body-large">
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">go compiler</VCol>
              <VCol class="text-start ps-2">{{ buildInfo.compiler }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">git commit</VCol>
              <VCol class="text-start ps-2">{{ buildInfo.commit }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">git ref</VCol>
              <VCol class="text-start ps-2">{{ buildInfo.ref }}</VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">built</VCol>
              <VCol class="text-start ps-2"><TimeAgo :value="buildInfo.buildTimeLocal" /></VCol>
            </VRow>
            <VRow no-gutters>
              <VCol class="text-medium-emphasis text-end pe-2">uptime</VCol>
              <VCol class="text-start ps-2"><TimeAgo :value="buildInfo.serverUptime" /></VCol>
            </VRow>
          </VCardText>
        </VCard>
      </VCol>
      <VCol cols="12" sm="6" lg="4">
        <VCard v-if="uiDeps.length">
          <VCardTitle class="text-medium-emphasis text-center">UI dependencies</VCardTitle>
          <VCardText class="text-body-large">
            <VRow v-for="pkg in uiDeps" :key="pkg.name" no-gutters>
              <VCol cols="7" class="text-end pe-2">
                <a
                  :href="`https://www.npmjs.com/package/${pkg.name}`"
                  target="_blank"
                  class="text-decoration-none"
                >
                  {{ pkg.name }}
                </a>
              </VCol>
              <VCol cols="5" class="text-start ps-2">
                {{ pkg.version }}
              </VCol>
            </VRow>
          </VCardText>
        </VCard>
      </VCol>
    </VRow>
  </VContainer>
</template>
