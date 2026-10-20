import { fileURLToPath, URL } from 'node:url';

import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vueDevTools from 'vite-plugin-vue-devtools';
import genDeps from './vite-plugin-gen-deps.ts';

// https://vite.dev/config/
export default defineConfig({
  base: '/ui/',
  plugins: [
    vue(),
    vueDevTools(),
    genDeps({
      beautify: false,
      includeDevDeps: ['typescript', 'vite'],
    }),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              test: /node_modules\/bootstrap-icons/,
              name: 'vendor-icons',
            },
            {
              test: /node_modules\/vuetify/,
              name: 'vendor-vuetify',
            },
            {
              test: /node_modules/,
              name: 'vendor',
            },
          ],
        },
      },
    },
  },
});
