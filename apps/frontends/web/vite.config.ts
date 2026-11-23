import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit({
    // 404.html renders the app's error page for unknown paths (see nginx.conf).
    adapter: adapter({ fallback: '404.html' }),
    // The app owns the site root; Notes is the /apps/notes/ route.
    paths: { relative: false },
    prerender: { crawl: false }
  })],
  server: { proxy: { '/api': { target: 'http://127.0.0.1:8080', rewrite: path => path.replace(/^\/api/, '') } } }
});
