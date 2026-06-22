import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// During `npm run dev`, /api is proxied to the Go server on :8080 so the
// browser talks to a single origin (no CORS needed). For production, run
// `npm run build` and let the Go server serve web/dist.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
