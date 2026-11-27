import { createRegistry, isRecord, nonEmpty, validId, type Registry, type Warn } from '../registry';
import type { AnyWidget, WidgetSize } from './types';

const size = (v: unknown): v is WidgetSize => isRecord(v) &&
  Number.isInteger(v.cols) && Number.isInteger(v.rows) && (v.cols as number) >= 1 && (v.rows as number) >= 1;

export function validateWidget(entry: unknown): string[] {
  if (!isRecord(entry)) return ['a widget definition must be an object'];
  const problems: string[] = [];
  const { id, name, description, defaultSize, minSize, maxSize, defaultConfig, component, settingsComponent, onMount, onUnmount } = entry;
  if (!validId(id)) problems.push('"id" must be a non-empty string of letters, digits, ".", "_", ":" or "-"');
  if (!nonEmpty(name)) problems.push('"name" is required');
  if (typeof description !== 'string') problems.push('"description" is required');
  if (!size(defaultSize)) problems.push('"defaultSize" must be { cols, rows } with positive integers');
  if (minSize !== undefined && !size(minSize)) problems.push('"minSize" must be { cols, rows } with positive integers');
  if (maxSize !== undefined && !size(maxSize)) problems.push('"maxSize" must be { cols, rows } with positive integers');
  if (size(defaultSize) && size(minSize) && (defaultSize.cols < minSize.cols || defaultSize.rows < minSize.rows))
    problems.push('"defaultSize" is smaller than "minSize"');
  if (size(defaultSize) && size(maxSize) && (defaultSize.cols > maxSize.cols || defaultSize.rows > maxSize.rows))
    problems.push('"defaultSize" is larger than "maxSize"');
  if (!isRecord(defaultConfig)) problems.push('"defaultConfig" must be an object');
  if (typeof component !== 'function') problems.push('"component" must be a Svelte component');
  if (settingsComponent !== undefined && typeof settingsComponent !== 'function') problems.push('"settingsComponent" must be a Svelte component');
  if (onMount !== undefined && typeof onMount !== 'function') problems.push('"onMount" must be a function');
  if (onUnmount !== undefined && typeof onUnmount !== 'function') problems.push('"onUnmount" must be a function');
  return problems;
}

export type WidgetRegistry = Registry<AnyWidget>;

export const createWidgetRegistry = (warn?: Warn): WidgetRegistry => createRegistry<AnyWidget>('widget', validateWidget, warn);

/** The widgets that can be placed on a board. */
export const widgetRegistry = createWidgetRegistry();

/** Clamps a size to a widget's limits (and to the 4-column board). */
export function clampSize(widget: AnyWidget, s: WidgetSize): WidgetSize {
  const min = widget.minSize ?? { cols: 1, rows: 1 }, max = widget.maxSize ?? { cols: 4, rows: 8 };
  const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(Math.round(v) || lo, lo), hi);
  return { cols: clamp(s.cols, min.cols, Math.min(max.cols, 4)), rows: clamp(s.rows, min.rows, max.rows) };
}
