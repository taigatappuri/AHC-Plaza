import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { resolve } from 'node:path'

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: resolve(__dirname, '../internal/server/static'),
    emptyOutDir: true
  }
})
