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

// vuetify's icon aliases default to mdi, which isn't loaded, so the ones the
// app relies on (VAlert's type icons) are mapped to bootstrap-icons.
const vuetify = createVuetify({
  theme: { defaultTheme: 'light' },
  icons: {
    aliases: {
      success: 'bi bi-check-circle text-title-large',
      info: 'bi bi-info-circle text-title-large',
      warning: 'bi bi-exclamation-triangle text-title-large',
      error: 'bi bi-exclamation-circle text-title-large',
    },
  },
});

// prettier-ignore
createApp(App)
  .use(router)
  .use(createPinia())
  .use(vuetify)
  .mount('#app');
