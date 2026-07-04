import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

const apiProxy = process.env.KEYHUB_API_PROXY || 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [react()],
  build: {
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks(id) {
          const normalizedID = id.replace(/\\/g, '/')
          if (!normalizedID.includes('/node_modules/')) {
            return undefined
          }
          if (normalizedID.includes('/node_modules/lucide-react/')) {
            return 'vendor-icons'
          }
          if (
            normalizedID.includes('/node_modules/react/') ||
            normalizedID.includes('/node_modules/react-dom/') ||
            normalizedID.includes('/node_modules/scheduler/')
          ) {
            return 'vendor-react'
          }
          if (
            normalizedID.includes('/node_modules/antd/') ||
            normalizedID.includes('/node_modules/@ant-design/') ||
            normalizedID.includes('/node_modules/@rc-component/') ||
            normalizedID.includes('/node_modules/rc-')
          ) {
            return 'vendor-antd'
          }
          return 'vendor'
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': apiProxy,
    },
  },
})
