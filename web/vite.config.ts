import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api/': {
        target: `http://localhost:${process.env.API_PORT ?? 8080}`,
        changeOrigin: false,
      },
    },
  },
})
