import { defineConfig } from '@vben/vite-config';

export default defineConfig(async () => ({
  application: {},
  vite: {
    // Vben is a pnpm workspace with vue-router imported by both the app and
    // framework packages. Force one runtime copy so useRouter() in layouts
    // receives the exact router instance installed by bootstrap.ts.
    resolve: {
      dedupe: ['vue', 'vue-router'],
    },
    server: {
      proxy: {
        '/api': {
          changeOrigin: true,
          target: 'http://localhost:38647',
        },
      },
    },
  },
}));
