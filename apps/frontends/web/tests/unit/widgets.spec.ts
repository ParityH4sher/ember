import { expect, test } from '@playwright/test';
import { clampSize, createWidgetRegistry } from '../../src/lib/widgets/registry';
import { addPlacement, changePlacement, parseLayout, removePlacement } from '../../src/lib/widgets/layout';
import type { WidgetContext, WidgetDefinition, WidgetProps } from '../../src/lib/widgets/types';

// A widget that is not the launcher and that nothing in Ember knows about,
// registered the same way a plugin would. The UI suite covers the board
// rendering the launcher, which the board likewise knows nothing about.
interface ClockConfig { timeZone: string }
const clockComponent = ((_anchor: unknown, _props: WidgetProps<ClockConfig>) => ({})) as unknown as WidgetDefinition<ClockConfig>['component'];
const clock: WidgetDefinition<ClockConfig> = {
  id: 'test.clock', name: 'Clock', description: 'Shows the time.',
  defaultSize: { cols: 1, rows: 1 }, minSize: { cols: 1, rows: 1 }, maxSize: { cols: 2, rows: 2 },
  defaultConfig: { timeZone: 'UTC' }, component: clockComponent
};

test.describe('widget registry', () => {
  test('a dummy widget registers, is listed and unregisters', () => {
    const warnings: string[] = [];
    const registry = createWidgetRegistry(m => warnings.push(m));
    const seen: string[][] = [];
    registry.subscribe(ws => seen.push(ws.map(w => w.id)));
    const undo = registry.register(clock);
    expect(registry.get('test.clock')).toBe(clock);
    undo();
    expect(seen).toEqual([[], ['test.clock'], []]);
    expect(warnings).toEqual([]);
  });

  test('lifecycle hooks receive the context', () => {
    const calls: string[] = [];
    const registry = createWidgetRegistry();
    registry.register({ ...clock, onMount: c => calls.push(`mount ${c.instanceId}`), onUnmount: c => calls.push(`unmount ${c.instanceId}`) });
    const context: WidgetContext = { instanceId: 'test.clock-1', displayId: 'main', theme: 'dark', launch: async () => true };
    const w = registry.get('test.clock')!;
    w.onMount?.(context);
    w.onUnmount?.(context);
    expect(calls).toEqual(['mount test.clock-1', 'unmount test.clock-1']);
  });

  test('bad widget definitions warn and are ignored', () => {
    const warnings: string[] = [];
    const registry = createWidgetRegistry(m => warnings.push(m));
    const bad: unknown[] = [
      { ...clock, id: undefined },
      { ...clock, component: 'Clock' },
      { ...clock, defaultSize: { cols: 0, rows: 1 } },
      { ...clock, defaultSize: { cols: 3, rows: 1 } },  // larger than maxSize
      { ...clock, defaultConfig: null },
      { ...clock, onMount: 'later' }
    ];
    for (const b of bad) registry.register(b as typeof clock);
    expect(registry.list()).toEqual([]);
    expect(warnings).toHaveLength(bad.length);
    expect(warnings[3]).toContain('larger than "maxSize"');
  });

  test('sizes are clamped to the widget limits and the 4-column board', () => {
    expect(clampSize(clock, { cols: 9, rows: 0 })).toEqual({ cols: 2, rows: 1 });
    expect(clampSize({ ...clock, maxSize: undefined }, { cols: 9, rows: 3 })).toEqual({ cols: 4, rows: 3 });
  });
});

test.describe('board layout', () => {
  test('a first visit places the initial widgets', () => {
    expect(parseLayout(null, ['ember.launcher'])).toEqual({
      version: 1, placements: [{ id: 'ember.launcher-1', widget: 'ember.launcher', config: {} }]
    });
  });

  test('a saved layout wins, even an empty one; junk is dropped', () => {
    expect(parseLayout({ version: 1, placements: [] }, ['ember.launcher']).placements).toEqual([]);
    const parsed = parseLayout({ version: 1, placements: [
      { id: 'a', widget: 'test.clock', size: { cols: 2, rows: 1 }, config: { timeZone: 'CET' } },
      { id: 'a', widget: 'test.clock' },                  // duplicate id
      { id: 'b', widget: 'x', size: { cols: -1 }, config: 'no' },
      'junk'
    ] }, []);
    expect(parsed.placements).toEqual([
      { id: 'a', widget: 'test.clock', size: { cols: 2, rows: 1 }, config: { timeZone: 'CET' } },
      { id: 'b', widget: 'x', size: undefined, config: {} }
    ]);
  });

  test('add, configure, resize and remove', () => {
    let l = parseLayout(null, []);
    l = addPlacement(l, 'test.clock');
    l = addPlacement(l, 'test.clock');
    expect(l.placements.map(p => p.id)).toEqual(['test.clock-1', 'test.clock-2']);
    l = changePlacement(l, 'test.clock-2', { config: { timeZone: 'CET' } });
    l = changePlacement(l, 'test.clock-2', { size: { cols: 2, rows: 2 } });
    expect(l.placements[1]).toEqual({ id: 'test.clock-2', widget: 'test.clock', size: { cols: 2, rows: 2 }, config: { timeZone: 'CET' } });
    l = removePlacement(l, 'test.clock-1');
    expect(l.placements.map(p => p.id)).toEqual(['test.clock-2']);
    expect(addPlacement(l, 'test.clock').placements.map(p => p.id)).toEqual(['test.clock-2', 'test.clock-1']);
  });
});
