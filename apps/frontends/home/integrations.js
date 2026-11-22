// The shell consumes Ember metadata. No plugin code or remote HTML is loaded.
import { account, signInUrl } from './account.js';

function el(tag, text) {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  return node;
}

async function api(path, method = 'GET', body) {
  const res = await fetch(path, {method, headers: {Accept: 'application/json',
    ...(body ? {'Content-Type': 'application/json'} : {})}, body: JSON.stringify(body)});
  if (!res.ok) {
    const error = await res.json().catch(() => ({}));
    throw new Error(error.message || `Ember returned HTTP ${res.status}`);
  }
  return res.json();
}

// Build controls from the same schema used for server-side validation.
function fields(parent, schema, values = {}, configured = false) {
  const readers = [];
  for (const [key, prop] of Object.entries(schema.properties || {})) {
    const required = schema.required?.includes(key);
    if (prop.type === 'object' && prop.additionalProperties === false && !Object.keys(prop.properties || {}).length) {
      if (required) readers.push([key, () => ({})]);
      continue;
    }
    if (prop.type === 'object' && prop.properties) {
      const group = el('fieldset');
      group.append(el('legend', key));
      parent.append(group);
      const read = fields(group, prop, values[key], configured);
      readers.push([key, () => {
        const obj = read();
        return required || Object.keys(obj).length ? obj : undefined;
      }]);
      continue;
    }
    const label = el('label', key + (required ? ' *' : ''));
    const numeric = ['integer', 'number'].includes(prop.type);
    const json = ['object', 'array'].includes(prop.type);
    const choices = prop.enum || (prop.type === 'boolean' ? [true, false] : null);
    const input = el(choices ? 'select' : json ? 'textarea' : 'input');
    if (choices) {
      input.append(new Option('Select…', ''));
      for (const choice of choices) input.append(new Option(String(choice), JSON.stringify(choice)));
    } else if (!json) {
      input.type = prop.writeOnly ? 'password' : numeric ? 'number' : prop.format === 'uri' ? 'url' : 'text';
      if (numeric) {
        input.step = prop.type === 'integer' ? '1' : 'any';
        if (prop.minimum !== undefined) input.min = prop.minimum;
        if (prop.maximum !== undefined) input.max = prop.maximum;
      }
    }
    input.required = Boolean(required && !(prop.writeOnly && configured));
    if (prop.writeOnly) {
      input.autocomplete = 'new-password';
      if (configured) input.placeholder = 'Leave blank to keep existing value';
    } else if (Object.hasOwn(values, key)) {
      input.value = choices || json ? JSON.stringify(values[key]) : values[key];
    }
    label.append(input);
    if (prop.description) label.append(el('small', prop.description));
    parent.append(label);
    readers.push([key, () => input.value === '' ? undefined :
      choices || json ? JSON.parse(input.value) : numeric ? input.valueAsNumber : input.value]);
  }
  return () => Object.fromEntries(readers.map(([key, read]) => [key, read()]).filter(([, v]) => v !== undefined));
}

function action(parent, title, fill, submit, disabled = false) {
  const form = el('form');
  const read = fill(form);
  const button = el('button', title);
  button.disabled = disabled;
  const status = el('pre');
  status.setAttribute('role', 'status');
  form.append(button, status);
  form.onsubmit = async event => {
    event.preventDefault();
    button.disabled = true;
    status.textContent = 'Working…';
    try { status.textContent = await submit(read()); }
    catch (error) { status.textContent = error.message; }
    finally { button.disabled = disabled; }
  };
  parent.append(form);
}

function card(plugin) {
  const section = el('article');
  const base = '/api/v1/plugins/' + encodeURIComponent(plugin.id);
  section.append(el('h3', plugin.name), el('p', plugin.description),
    el('p', `${plugin.active ? 'Active' : 'Inactive'} · ${plugin.configured ? 'Configured' : 'Needs configuration'} · ${plugin.version}`));
  const settings = el('details');
  settings.append(el('summary', 'Settings'));
  action(settings, 'Save settings', f => fields(f, plugin.configurationSchema, plugin.configuration, plugin.configured), async values => {
    await api(base + '/configuration', 'PUT', {values});
    section.replaceWith(card(await api(base)));
    return 'Saved';
  });
  section.append(settings);
  action(section, plugin.active ? 'Deactivate' : 'Activate', () => () => null, async () => {
    section.replaceWith(card(await api(base + '/activation', 'PUT', {active: !plugin.active})));
    return 'Updated';
  }, !plugin.configured);
  const ops = el('details');
  ops.append(el('summary', 'Operations'));
  section.append(ops);
  let loading = false;
  const list = el('div');
  ops.append(list);
  ops.ontoggle = async () => {
    if (!ops.open || loading) return;
    loading = true;
    list.textContent = 'Loading…';
    try {
      const catalog = await api(base + '/api');
      list.textContent = catalog.operations?.length ? '' : 'No operations advertised.';
      for (const op of catalog.operations || []) {
        const path = `${base}/api/v${encodeURIComponent(catalog.apiVersion)}/${encodeURIComponent(op.operationId)}`;
        list.append(el('h4', op.name), el('p', op.description));
        // Use only the expected same-origin route, never a catalog-supplied destination.
        if (op.method !== 'POST' || path !== op.path) {
          list.append(el('p', 'Unsupported operation route.'));
          continue;
        }
        action(list, `Run ${op.name}`, f => fields(f, op.inputSchema), async input =>
          JSON.stringify((await api(path, 'POST', input)).data, null, 2), !plugin.active);
      }
    } catch (err) { list.textContent = err.message; loading = false; }
  };
  return section;
}

async function load(id, render) {
  const host = document.getElementById(id);
  try {
    const items = (await api('/api/v1/' + id))[id];
    host.replaceChildren(...items.map(render));
    if (!items.length) host.textContent = `No ${id} configured.`;
  } catch (err) { host.textContent = `Unable to load ${id}: ${err.message}`; }
}

if (await account(document.getElementById('account'))) {
  load('plugins', card);
  load('services', renderService);
} else {
  for (const id of ['plugins', 'services']) {
    const link = el('a', 'Sign in');
    link.href = signInUrl();
    document.getElementById(id).replaceChildren(link, ` to view ${id}.`);
  }
}

function renderService(service) {
  const row = el('p');
  const url = new URL(service.url);
  if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) return row;
  const link = el('a', `Open ${service.name} →`);
  link.href = url.href;
  link.referrerPolicy = 'no-referrer';
  row.append(link);
  return row;
}
