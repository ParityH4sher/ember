import { expect, test } from '@playwright/test';
import { createAppRegistry, validateApp } from '../../src/lib/apps/registry';
import { launchApp } from '../../src/lib/apps/launch';
import type { AppDefinition } from '../../src/lib/apps/types';

const notes: AppDefinition = { id: 'notes', name: 'Notes', launch: { type: 'internal', path: '/apps/notes/' } };
const docs: AppDefinition = { id: 'docs', name: 'Docs', launch: { type: 'url', url: 'https://docs.example.com/' } };

function setup() {
  const warnings: string[] = [];
  return { registry: createAppRegistry(m => warnings.push(m)), warnings };
}

test.describe('app registry', () => {
  test('registers, lists in order, gets and unregisters', () => {
    const { registry, warnings } = setup();
    registry.register(notes);
    registry.register(docs);
    expect(registry.list().map(a => a.id)).toEqual(['notes', 'docs']);
    expect(registry.get('docs')).toBe(docs);
    expect(registry.unregister('notes')).toBe(true);
    expect(registry.unregister('notes')).toBe(false);
    expect(registry.list().map(a => a.id)).toEqual(['docs']);
    expect(warnings).toEqual([]);
  });

  test('notifies subscribers at once and on every change', () => {
    const { registry } = setup();
    const seen: string[][] = [];
    const stop = registry.subscribe(apps => seen.push(apps.map(a => a.id)));
    const undo = registry.register(notes);
    undo();
    stop();
    registry.register(docs);
    expect(seen).toEqual([[], ['notes'], []]);
  });

  test('ignores a duplicate id with a warning, unless replacing', () => {
    const { registry, warnings } = setup();
    registry.register(notes);
    const other = { ...notes, name: 'Other notes' };
    registry.register(other);
    expect(registry.get('notes')).toBe(notes);
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain('"notes"');
    expect(warnings[0]).toContain('already registered');
    registry.register(other, { replace: true });
    expect(registry.get('notes')).toBe(other);
    expect(registry.list()).toHaveLength(1);
  });

  test('registering the same object twice is harmless', () => {
    const { registry, warnings } = setup();
    registry.register(notes);
    registry.register(notes);
    expect(registry.list()).toHaveLength(1);
    expect(warnings).toEqual([]);
  });

  test('a disposer removes only the entry it registered', () => {
    const { registry } = setup();
    const undo = registry.register(notes);
    const replacement = { ...notes, name: 'New notes' };
    registry.register(replacement, { replace: true });
    undo();
    expect(registry.get('notes')).toBe(replacement);
  });

  test('bad definitions warn and are ignored, never throw', () => {
    const { registry, warnings } = setup();
    const bad: unknown[] = [
      null, 'notes', {},
      { id: '', name: 'No id', launch: { type: 'internal', path: '/' } },
      { id: 'no-name', launch: { type: 'internal', path: '/' } },
      { id: 'no-launch', name: 'No launch' },
      { id: 'bad-type', name: 'Bad type', launch: { type: 'ftp' } },
      { id: 'offsite', name: 'Off-site', launch: { type: 'internal', path: '//evil.example/' } },
      { id: 'script', name: 'Script', launch: { type: 'url', url: 'javascript:alert(1)' } },
      { id: 'creds', name: 'Creds', launch: { type: 'url', url: 'https://user:pw@example.com/' } },
      { id: 'no-run', name: 'No run', launch: { type: 'custom' } },
      { id: 'icon', name: 'Icon', icon: 'javascript:alert(1)', launch: { type: 'internal', path: '/' } },
      { id: 'keywords', name: 'Keywords', keywords: 'notes', launch: { type: 'internal', path: '/' } },
      { id: 'has space', name: 'Space', launch: { type: 'internal', path: '/' } }
    ];
    for (const b of bad) expect(() => registry.register(b as AppDefinition)).not.toThrow();
    expect(registry.list()).toEqual([]);
    expect(warnings).toHaveLength(bad.length);
    expect(warnings.find(w => w.includes('"no-name"'))).toContain('"name" is required');
    expect(warnings.find(w => w.includes('"script"'))).toContain('http(s)');
  });

  test('accepts every icon and launch form', () => {
    const fn = () => {};
    const ok: unknown[] = [
      { id: 'a', name: 'A', icon: '/icons/a.svg', launch: { type: 'internal', path: '/a/' } },
      { id: 'b', name: 'B', icon: 'https://cdn.example.com/b.png', launch: { type: 'url', url: 'https://b.example/', newWindow: false } },
      { id: 'c', name: 'C', icon: 'data:image/svg+xml;base64,PHN2Zy8+', launch: { type: 'custom', run: fn } },
      { id: 'd', name: 'D', icon: fn, category: 'Tools', keywords: ['x'], launch: { type: 'custom', run: fn } },
      { id: 'e', name: 'E', icon: { component: fn, props: { name: 'notes' } }, launch: { type: 'internal', path: '/' } }
    ];
    for (const app of ok) expect(validateApp(app)).toEqual([]);
  });

  test('a validator that throws counts as invalid', () => {
    const { registry, warnings } = setup();
    const evil = { get id(): string { throw new Error('boom'); } };
    expect(() => registry.register(evil as unknown as AppDefinition)).not.toThrow();
    expect(warnings).toHaveLength(1);
  });
});

test.describe('launching', () => {
  const openers = () => {
    const calls: string[] = [];
    return { calls, openers: {
      openInternal: (p: string) => { calls.push(`internal ${p}`); },
      openUrl: (u: string, w: boolean) => { calls.push(`url ${u} ${w ? 'new' : 'same'}`); }
    } };
  };

  test('dispatches each target type', async () => {
    const { calls, openers: o } = openers();
    let ran = '';
    expect(await launchApp(notes, o)).toBe(true);
    expect(await launchApp(docs, o)).toBe(true);
    expect(await launchApp({ ...docs, launch: { type: 'url', url: 'https://x.example/', newWindow: false } }, o)).toBe(true);
    expect(await launchApp({ id: 'c', name: 'C', launch: { type: 'custom', run: app => { ran = app.id; } } }, o)).toBe(true);
    expect(calls).toEqual(['internal /apps/notes/', 'url https://docs.example.com/ new', 'url https://x.example/ same']);
    expect(ran).toBe('c');
  });

  test('a failing launch resolves false with a warning', async () => {
    const warnings: string[] = [];
    const app: AppDefinition = { id: 'broken', name: 'Broken', launch: { type: 'custom', run: async () => { throw new Error('nope'); } } };
    expect(await launchApp(app, openers().openers, m => warnings.push(m))).toBe(false);
    expect(warnings[0]).toContain('"broken"');
    expect(warnings[0]).toContain('nope');
  });
});
