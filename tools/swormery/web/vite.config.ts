import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

// Dev proxy targets the Go daemon on its default port.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    // Keep web/dist/.gitkeep (required by go:embed on fresh clones).
    emptyOutDir: false,
  },
  server: {
    proxy: {
      '/api': `http://localhost:${process.env.SWORMERY_PORT ?? '7777'}`,
    },
  },
});
