import { createServer } from '../../apps/frontends/web/node_modules/vite/dist/node/index.js';

const allowedHosts = process.env.FIGMA_PUBLIC_URL
  ? [new URL(process.env.FIGMA_PUBLIC_URL).hostname]
  : [];

const server = await createServer({
  server: {
    host: '0.0.0.0',
    port: Number(process.env.PORT),
    strictPort: true,
    allowedHosts,
    proxy: {
      '/api': {
        target: `http://127.0.0.1:${process.env.EMBER_MAKE_API_PORT}`,
        rewrite: path => path.replace(/^\/api/, ''),
      },
    },
  },
});
await server.listen();
server.printUrls();
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, async () => {
    await server.close();
    process.exit(0);
  });
}
