import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

const backend = process.env.KAPOJ_BACKEND_URL ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    proxy: {
      '/api': backend,
      '/health': backend,
    },
    warmup: {
      clientFiles: [
        './src/main.ts',
        './src/App.vue',
        './src/router/index.ts',
        './src/layouts/DefaultLayout.vue',
        './src/layouts/AdminLayout.vue',
        './src/views/HomeView.vue',
        './src/views/LoginView.vue',
        './src/views/ProblemListView.vue',
        './src/views/ProblemDetailView.vue',
        './src/views/SubmitView.vue',
        './src/views/SubmissionsView.vue',
        './src/views/ContestListView.vue',
        './src/views/ContestDetailView.vue',
        './src/views/LeaderboardView.vue',
      ],
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.spec.ts'],
    setupFiles: ['src/test/setup.ts'],
  },
})
