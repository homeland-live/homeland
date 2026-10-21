import { createRouter, createWebHashHistory } from 'vue-router';
import DevicesView from '@/views/DevicesView.vue';
import DeviceView from '@/views/DeviceView.vue';
import ScriptsView from '@/views/ScriptsView.vue';
import BridgeView from '@/views/BridgeView.vue';
import StatusBuildView from '@/views/StatusBuildView.vue';
import StatusHealthView from '@/views/StatusHealthView.vue';

const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  linkActiveClass: 'active',
  routes: [
    { path: '/', name: 'IndexView', redirect: { name: 'DevicesView' } },
    { path: '/devices', name: 'DevicesView', component: DevicesView },
    { path: '/devices/:id', name: 'DeviceView', component: DeviceView },
    { path: '/scripts', name: 'ScriptsView', component: ScriptsView },
    { path: '/bridge', name: 'BridgeView', component: BridgeView },
    { path: '/status/build', name: 'StatusBuildView', component: StatusBuildView },
    { path: '/status/health', name: 'StatusHealthView', component: StatusHealthView },
  ],
});

export default router;
