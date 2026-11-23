<script lang="ts">
  import SchemaFields from './SchemaFields.svelte';
  import { choices, fieldName, initialValue, kind, nested, type Schema } from './schema';

  let { schema, values = {}, configured = false, path = [] }: {
    schema: Schema;
    /** Stored values to show; write-only ones are never returned by the API. */
    values?: Record<string, unknown>;
    /** A stored configuration exists, so a blank write-only field keeps its value. */
    configured?: boolean;
    path?: string[];
  } = $props();
</script>

{#each Object.entries(schema.properties ?? {}) as [key, prop] (key)}
  {@const k = kind(prop)}
  {@const name = fieldName([...path, key])}
  {@const required = (schema.required?.includes(key) ?? false) && !(prop.writeOnly && configured)}
  {@const value = initialValue(prop, values, key)}
  {#if k === 'group'}
    <fieldset>
      <legend>{key}</legend>
      <SchemaFields schema={prop} values={nested(values, key)} {configured} path={[...path, key]} />
    </fieldset>
  {:else if k !== 'omit'}
    <label>{key}{schema.required?.includes(key) ? ' *' : ''}
      {#if k === 'choice'}
        <select {name} {required} {value}>
          <option value="">Select…</option>
          {#each choices(prop) as choice}
            <option value={JSON.stringify(choice)}>{String(choice)}</option>
          {/each}
        </select>
      {:else if k === 'json'}
        <textarea {name} {required} {value}></textarea>
      {:else}
        <input {name} {required} {value}
          type={prop.writeOnly ? 'password' : k === 'number' ? 'number' : prop.format === 'uri' ? 'url' : 'text'}
          step={k === 'number' ? (prop.type === 'integer' ? '1' : 'any') : undefined}
          min={prop.minimum} max={prop.maximum}
          autocomplete={prop.writeOnly ? 'new-password' : undefined}
          placeholder={prop.writeOnly && configured ? 'Leave blank to keep existing value' : undefined}>
      {/if}
      {#if prop.description}<small>{prop.description}</small>{/if}
    </label>
  {/if}
{/each}

<style>
  fieldset { margin: .8rem 0; border: 1px solid var(--border); border-radius: .3rem }
  small { display: block; color: var(--muted) }
</style>
