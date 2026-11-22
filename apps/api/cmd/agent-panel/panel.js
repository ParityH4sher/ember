const el = id => document.getElementById(id);
const request = async (route, body) => {
  const options = body === undefined ? {} : {
    method: 'POST', headers: {'X-Ember-Action': '1', 'Content-Type': 'application/json'},
    body: JSON.stringify(body)
  };
  const response = await fetch('./' + route, options);
  if (!response.ok) throw new Error(`${route}: ${response.status}`);
  if (response.headers.get('Content-Type')?.includes('application/json')) return response.json();
};
const report = error => { el('notice').textContent = error.message; };
let revision = 0;
async function refresh() {
  const current = ++revision;
  const [messages, permissions] = await Promise.all([request('messages'), request('permissions')]);
  if (current !== revision) return;
  el('history').textContent = messages.map(m => `${m.role}: ${m.text}`).join('\n\n');
  el('permissions').replaceChildren(...permissions.map(p => {
    const box = document.createElement('p');
    box.textContent = `Permission requested: ${p.action} — ${p.resources.join(', ')} `;
    for (const [action, label] of [['once', 'Allow once'], ['reject', 'Reject']]) {
      const button = document.createElement('button');
      button.textContent = label;
      button.onclick = async () => {
        try { await request(`permissions/${encodeURIComponent(p.id)}/${action}`, {}); await refresh(); }
        catch (error) { report(error); }
      };
      box.append(button);
    }
    return box;
  }));
}
el('form').onsubmit = async event => {
  event.preventDefault();
  const text = el('prompt').value;
  if (!text.trim()) return;
  el('send').disabled = true;
  try {
    await request('steer', {text});
    if (el('prompt').value === text) el('prompt').value = '';
    el('notice').textContent = 'Message accepted.';
    await refresh();
  } catch (error) { report(error); }
  finally { el('send').disabled = false; }
};
el('stop').onclick = async () => {
  try {
    const result = await request('interrupt', {});
    el('notice').textContent = result.interrupted ? 'Execution interrupted.' : 'No active execution was interrupted.';
    await refresh();
  } catch (error) { report(error); }
};
function subscribe() {
  const events = new EventSource('./events');
  events.onerror = () => {
    el('connection').textContent = 'Live updates disconnected. Reconnecting…';
    // Native EventSource retries a dropped stream, but not an initial HTTP
    // failure while the worker is still coming back up.
    if (events.readyState === EventSource.CLOSED) setTimeout(subscribe, 3000);
  };
  events.onmessage = ({data}) => {
    const event = JSON.parse(data);
    if (event.type === 'session.text.delta') {
      el('live').textContent = (el('live').textContent + event.delta).slice(-16000);
    } else {
      el('live').textContent = '';
      el('connection').textContent = event.type === 'connected' ? 'Connected' : event.type.replace(/^session\./, '').replaceAll('.', ' ');
      refresh().catch(report);
    }
  };
}
try {
  await request('connect', {});
  await refresh();
  el('send').disabled = el('stop').disabled = false;
  subscribe();
} catch (error) {
  el('connection').textContent = 'Could not open the task. Check the worker and reload.';
  report(error);
}
