import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit({
    adapter: adapter(),
    paths: { base: '/apps/notes', relative: false },
    prerender: { crawl: false }
  })],
  server: { proxy: { '/api': { target: 'http://127.0.0.1:8080', rewrite: path => path.replace(/^\/api/, '') } } }
});
