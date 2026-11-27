import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/admin/api': 'http://127.0.0.1:6760',
      '/v1': 'http://127.0.0.1:6760',
      '/healthz': 'http://127.0.0.1:6760',
    },
  },
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
  },
})
