import { spawn } from 'node:child_process';
import { unlinkSync } from 'node:fs';
import { resolve } from 'node:path';
import { createServer } from 'node:net';

// Keep both services supervised and terminate the sibling if either exits.
const binary = process.argv[2];
const children = new Set();
let stopping = false;
let killTimer;

function finish() {
  if (!stopping || children.size) return;
  clearTimeout(killTimer);
  try { unlinkSync(binary); } catch {}
}

function stop(code = 0) {
  if (stopping) return;
  stopping = true;
  process.exitCode = code;
  for (const child of children) child.kill('SIGTERM');
  killTimer = setTimeout(() => {
    for (const child of children) child.kill('SIGKILL');
  }, 7000);
  killTimer.unref();
  finish();
}

function start(command, args, options) {
  const child = spawn(command, args, { stdio: 'inherit', ...options });
  children.add(child);
  child.on('error', error => {
    console.error(error.message);
    children.delete(child);
    stop(1);
    finish();
  });
  child.on('exit', (code, signal) => {
    children.delete(child);
    if (!stopping) {
      console.error(`${command} exited (${signal || code})`);
      stop(code || 1);
    }
    finish();
  });
  return child;
}

process.on('SIGTERM', () => stop());
process.on('SIGINT', () => stop());

if (!process.env.PORT || !binary) {
  console.error('PORT and the compiled API binary are required');
  process.exit(1);
}

const reservation = createServer();
await new Promise((done, reject) => {
  reservation.once('error', reject);
  reservation.listen(0, '127.0.0.1', done);
});
const apiPort = reservation.address().port;
await new Promise(done => reservation.close(done));
const apiEnv = {
  ...process.env,
  EMBER_LISTEN_ADDR: `127.0.0.1:${apiPort}`,
  EMBER_PROJECT_ROOT: process.env.EMBER_PROJECT_ROOT || resolve('apps/api/projects'),
};
start(binary, [], { cwd: resolve('apps/api'), env: apiEnv });

try {
  let ready = false;
  for (let attempt = 0; attempt < 100 && !stopping; attempt++) {
    try {
      const response = await fetch(`http://127.0.0.1:${apiPort}/healthz`, {
        signal: AbortSignal.timeout(1000),
      });
      if (response.ok) { ready = true; break; }
    } catch {}
    await new Promise(done => setTimeout(done, 100));
  }
  if (!ready && !stopping) throw new Error('Ember API did not become healthy');
  if (ready && !stopping) {
    const env = { ...process.env, EMBER_MAKE_API_PORT: String(apiPort) };
    start(process.execPath, [
      resolve('.figma/make/ember-vite.mjs'),
    ], { cwd: resolve('apps/frontends/web'), env });
  }
} catch (error) {
  console.error(error.message);
  stop(1);
}
