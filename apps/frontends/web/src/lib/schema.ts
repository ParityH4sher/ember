// Builds form controls from the same JSON Schema subset the API validates
// against (apps/api/internal/httpapi/schema.go). Controls are uncontrolled:
// SchemaFields.svelte renders them, and readFields reads the submitted form.

export interface Schema {
  type?: string;
  properties?: Record<string, Schema>;
  required?: string[];
  additionalProperties?: unknown;
  enum?: unknown[];
  format?: string;
  writeOnly?: boolean;
  minimum?: number;
  maximum?: number;
  description?: string;
}

/**
 * How a property is shown. `omit` is an object that cannot hold any field: it
 * renders nothing and is sent as `{}` when required. `group` is a fieldset of
 * nested fields. `choice` (an enum or a boolean) and `json` (any other object
 * or array) hold JSON text.
 */
export type Kind = 'omit' | 'group' | 'choice' | 'json' | 'number' | 'text';

export function kind(prop: Schema): Kind {
  if (prop.type === 'object' && prop.additionalProperties === false && !Object.keys(prop.properties ?? {}).length) {
    return 'omit';
  }
  if (prop.type === 'object' && prop.properties) return 'group';
  if (prop.enum || prop.type === 'boolean') return 'choice';
  if (prop.type === 'object' || prop.type === 'array') return 'json';
  if (prop.type === 'integer' || prop.type === 'number') return 'number';
  return 'text';
}

export const choices = (prop: Schema): unknown[] => prop.enum ?? [true, false];

/** A form control name that cannot collide for any property path. */
export const fieldName = (path: string[]) => JSON.stringify(path);

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

/** The stored value of a nested object, or an empty one. */
export const nested = (values: unknown, key: string): Record<string, unknown> =>
  isRecord(values) && isRecord(values[key]) ? values[key] : {};

/** A control's initial text. Write-only (secret) values are never shown. */
export function initialValue(prop: Schema, values: Record<string, unknown>, key: string): string {
  if (prop.writeOnly || !Object.hasOwn(values, key)) return '';
  const k = kind(prop);
  return k === 'choice' || k === 'json' ? JSON.stringify(values[key]) : String(values[key]);
}

/**
 * Reads the values of schema's fields from a form rendered by SchemaFields.
 * Empty controls are left out, so the API keeps a stored write-only value that
 * was not retyped. Throws on JSON text that does not parse.
 */
export function readFields(schema: Schema, form: HTMLFormElement, path: string[] = []): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, prop] of Object.entries(schema.properties ?? {})) {
    const required = schema.required?.includes(key) ?? false;
    const here = [...path, key];
    const k = kind(prop);
    let value: unknown;
    if (k === 'omit') {
      value = required ? {} : undefined;
    } else if (k === 'group') {
      const group = readFields(prop, form, here);
      value = required || Object.keys(group).length ? group : undefined;
    } else {
      const control = form.elements.namedItem(fieldName(here));
      const text = control instanceof HTMLInputElement || control instanceof HTMLSelectElement ||
        control instanceof HTMLTextAreaElement ? control.value : '';
      value = text === '' ? undefined
        : k === 'choice' || k === 'json' ? JSON.parse(text)
        : k === 'number' ? Number(text) : text;
    }
    if (value !== undefined) out[key] = value;
  }
  return out;
}
