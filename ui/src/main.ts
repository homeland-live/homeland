// vuetify
import 'vuetify/styles';
// icons
import 'bootstrap-icons/font/bootstrap-icons.min.css';
// app
import '@/assets/main.css';

import { createApp } from 'vue';
import { createPinia } from 'pinia';

import { createVuetify } from 'vuetify';

import App from '@/App.vue';
import router from '@/router';

// prettier-ignore
createApp(App)
  .use(router)
  .use(createPinia())
  .use(createVuetify({ theme: { defaultTheme: 'light' } }))
  .mount('#app');
