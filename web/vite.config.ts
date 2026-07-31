import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// Build the SPA straight into the Go embed directory.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: '../internal/webserver/dist',
    emptyOutDir: true,
  },
  test: {
    include: ['src/**/*.test.ts'],
  },
})
