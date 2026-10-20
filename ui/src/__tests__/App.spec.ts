import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ref } from 'vue';
import { mount } from '@vue/test-utils';
import { createVuetify } from 'vuetify';
import { createRouter, createMemoryHistory, type Router } from 'vue-router';
import { createPinia } from 'pinia';
import App from '@/App.vue';

// Control Vuetify's display composable so we can drive mobile vs desktop.
const mobile = ref(false);
vi.mock('vuetify', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vuetify')>();
  return { ...actual, useDisplay: () => ({ mobile }) };
});

function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'IndexView', redirect: { name: 'DevicesView' } },
      { path: '/devices', name: 'DevicesView', component: { template: '<div />' } },
      { path: '/scripts', name: 'ScriptsView', component: { template: '<div />' } },
      { path: '/bridge', name: 'BridgeView', component: { template: '<div />' } },
      { path: '/status/build', name: 'StatusBuildView', component: { template: '<div />' } },
    ],
  });
}

async function mountApp() {
  const router = makeRouter();
  await router.push('/devices');
  await router.isReady();

  const wrapper = mount(App, {
    global: {
      plugins: [createVuetify(), router, createPinia()],
      stubs: { 'router-view': true },
    },
  });
  return wrapper;
}

beforeEach(() => {
  mobile.value = false;
});

describe('App', () => {
  it('renders the Homeland brand inside the navigation drawer', async () => {
    const wrapper = await mountApp();
    const drawer = wrapper.find('.v-navigation-drawer');
    expect(drawer.exists()).toBe(true);
    expect(drawer.text()).toContain('Homeland');
    expect(drawer.find('.bi-house-gear').exists()).toBe(true);
  });

  it('renders the navigation items', async () => {
    const wrapper = await mountApp();
    const text = wrapper.text();
    expect(text).toContain('Devices');
    expect(text).toContain('Scripts');
    expect(text).toContain('Bridge');
    expect(text).toContain('BuildInfo');
  });

  it('does not render an app bar', async () => {
    const wrapper = await mountApp();
    expect(wrapper.find('.v-app-bar').exists()).toBe(false);
  });

  it('on desktop renders a permanent drawer and no toggle button', async () => {
    mobile.value = false;
    const wrapper = await mountApp();
    expect(wrapper.find('.app-nav-toggle').exists()).toBe(false);
    expect(wrapper.find('.v-navigation-drawer--is-mobile').exists()).toBe(false);
  });

  it('on mobile renders the toggle and opens the drawer when clicked', async () => {
    mobile.value = true;
    const wrapper = await mountApp();

    const toggle = wrapper.find('.app-nav-toggle');
    expect(toggle.exists()).toBe(true);

    // Drawer starts closed on mobile.
    const drawerVm = wrapper.getComponent(
      await import('vuetify/components/VNavigationDrawer').then((m) => m.VNavigationDrawer),
    );
    expect(drawerVm.props('modelValue')).toBe(false);

    await toggle.trigger('click');
    expect(drawerVm.props('modelValue')).toBe(true);
  });
});
