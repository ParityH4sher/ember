---
sidebar_label: Models and workers
sidebar_position: 7
---

# Models, the model gateway and OpenCode workers

Ember does not hold model provider keys. Providers have one owner, the model
gateway ([LiteLLM](https://docs.litellm.ai/), selected in
[DEC-007](planning/human-decisions.md#dec-007---model-routing-privacy-and-spending)),
as decided in [ADR 015](decisions/015-harness-sessions.md):

```text
admin page ── lists models ──> Ember API ── GET /v1/models ──> LiteLLM ──> providers
                                   │                              ▲
                                   └── session API ──> OpenCode ──┘ (virtual key)
```

- **LiteLLM** (`deploy/compose.models.yaml`) holds provider endpoints and keys
  in its own configuration and secrets directory.
- **Ember** lists the gateway's models for admins, who offer some of them under
  a display name and map agents to them (**Admin → Agents & models**). The
  catalog file stores no keys.
- **An OpenCode worker** (`deploy/compose.opencode.yaml`) runs session agents.
  It reaches models only through LiteLLM, with a virtual key limited to its
  model and a budget.

Status: the overlays are new and have not yet been qualified on the target
Raspberry Pi; DEC-007's per-user routing, budgets and MCP access are not
implemented. The Ansible `ember` role does not add these overlays yet; see
[Ansible](#ansible).

## Before you start

- A host running Ember with sign-in ([Sign-in (OIDC)](authentication.md)),
  installed from the source bundle with `install.sh` or Compose directly.
- Root on the host (the secret files are owned by container UIDs).
- For a worker: a Docker host with a qualified `io.containerd.kata.v2`
  runtime (see the
  [sandbox router prerequisites](https://github.com/ParityH4sher/ember/tree/main/services/sandbox-router)).
  Agents run tools and generated code in the worker, so keep the VM boundary.

The commands below use `/etc/ember` for host files and run from the extracted
bundle directory (or the source checkout root).

## 1. Configure providers in LiteLLM

Copy the example configuration and edit `model_list`. Each `model_name` is the
name Ember lists and offers, and the name agents ask for. Never write a key in
this file: name it as `os.environ/NAME`.

```sh
sudo install -d -m 0755 /etc/ember/litellm
sudo install -m 0644 deploy/litellm/config.example.yaml /etc/ember/litellm/config.yaml
sudoedit /etc/ember/litellm/config.yaml
```

Create the secrets directory. Each file is one environment variable for
LiteLLM: the file name is the variable name, the first line its value.
LiteLLM runs as UID 65534; Postgres (UID 70) also reads `POSTGRES_PASSWORD`.

```sh
dir=/etc/ember/litellm/secrets
sudo install -d -o 65534 -g 65534 -m 0700 "$dir"
printf 'sk-%s\n' "$(openssl rand -hex 32)" | sudo install -o 65534 -g 65534 -m 0400 /dev/stdin "$dir/LITELLM_MASTER_KEY"
openssl rand -hex 32 | sudo install -o 65534 -g 70 -m 0440 /dev/stdin "$dir/POSTGRES_PASSWORD"
# One file per key named in config.yaml. read -s keeps the key off the screen
# and out of the shell history.
read -rs -p 'Anthropic API key: ' key && echo &&
  printf '%s\n' "$key" | sudo install -o 65534 -g 65534 -m 0400 /dev/stdin "$dir/ANTHROPIC_API_KEY"; unset key
```

Start LiteLLM and its database on their own first, to create keys:

```sh
export EMBER_LITELLM_CONFIG_FILE=/etc/ember/litellm/config.yaml
export EMBER_LITELLM_SECRETS_DIR=/etc/ember/litellm/secrets
export EMBER_MODEL_GATEWAY_KEY_FILE=/etc/ember/model-gateway-key
boot="docker compose -f deploy/compose.yaml -f deploy/compose.models.yaml"
$boot up -d litellm
$boot ps litellm litellm-db     # wait until litellm is healthy (up to a minute)
$boot logs litellm              # check that every model in config.yaml loaded
```

LiteLLM has no published port. It reaches providers over the `models-egress`
network, the only network with a route out; a model on your LAN needs an
address the container can resolve, not `localhost`.

## 2. Connect Ember to the gateway

The API needs a key to call `GET /v1/models`. Create a LiteLLM virtual key for
it (the master key also works, but it can administer the gateway):

```sh
$boot exec -T litellm python - ember-api < deploy/litellm/new-key.py |
  sudo install -o 65532 -g 65532 -m 0400 /dev/stdin "$EMBER_MODEL_GATEWAY_KEY_FILE"
```

Then install with the three variables still exported:

```sh
./install.sh --check
./install.sh
```

`install.sh` adds `deploy/compose.models.yaml` when `EMBER_LITELLM_CONFIG_FILE`
is set. Without the installer, add `-f deploy/compose.models.yaml` to your
Compose command. The API gets `EMBER_MODEL_GATEWAY_URL=http://litellm:4000`
and `EMBER_MODEL_GATEWAY_KEY_FILE`; it reads the key once at startup, so
restart the API after replacing it.

**Upgrading from a catalog with providers.** The first start after the upgrade
rewrites `catalog.json` as `ember.ai-catalog/v2`: providers and their API keys
are removed, model IDs, names and agents are kept, and the API logs
`AI catalog migrated` with the number of discarded keys. Configure those
providers in LiteLLM, and give each kept model a LiteLLM `model_name` equal to
its old Ember ID. Rotate the discarded keys at the provider: renaming the file
does not erase old disk blocks, and backups still hold them.

## 3. Offer models

Open **Admin → Agents & models**. With the gateway connected, the page lists
the gateway's models (`GET /v1/admin/ai/gateway/models`). Offer one by giving
it a display name; its ID is the gateway's `model_name`. If the gateway is not
configured or not reachable, the page shows `gateway_unavailable` and a fixed
reason; the gateway's own error text is never passed on.

From the command line, as an admin with a session cookie:

```sh
curl -fsS -b "$cookie" https://ember.example/api/v1/admin/ai/gateway/models
curl -fsS -b "$cookie" -X PUT -H 'Content-Type: application/json' \
  -H 'Origin: https://ember.example' -d '{"name":"Claude Sonnet"}' \
  https://ember.example/api/v1/admin/ai/models/claude-sonnet
```

A model name with a `/` (for example `openai/gpt-4o`) is sent as `%2F` in the
path.

## 4. Map agents

An agent is one of two kinds:

| Kind | Harness | Set | Runs |
| --- | --- | --- | --- |
| Turn agent | `claude-code`, `codex`, `custom` | `routerAgent` (sandbox router policy name) | once per message in the [sandbox router](project-chat.md) |
| Session agent | `opencode` | `worker` (a worker ID from the agents file) | as one OpenCode session per chat |

A turn agent's default model is passed in its `ember.chat-turn/v1` envelope as
`{"id", "name"}`; the agent asks the gateway for that `id`. A session agent's
model is the worker's `EMBER_OPENCODE_MODEL` for now (ADR 015).

## 5. Run an OpenCode worker

Create the worker's password, shared by the API and the worker (both UID
65532), and a virtual key limited to the worker's model with a budget (here
20 USD per 30 days):

```sh
openssl rand -hex 32 | sudo install -o 65532 -g 65532 -m 0400 /dev/stdin /etc/ember/opencode-password
$boot exec -T litellm python - opencode-worker claude-sonnet 20 < deploy/litellm/new-key.py |
  sudo install -m 0400 /dev/stdin /etc/ember/opencode-gateway-key
```

Add the worker to the agents file (`EMBER_AGENTS_CONFIG_FILE`), and a session
agent if you do not add one from the admin page:

```json
{
  "workers": [
    {"id": "opencode", "harness": "opencode", "url": "http://opencode-worker:4096",
     "passwordFile": "/run/secrets/opencode_password", "directory": "/workspace"}
  ],
  "agents": [
    {"id": "coder", "name": "Coder", "worker": "opencode"}
  ]
}
```

`routerSocket` is needed only when the file also lists router agents. Then
install with the worker variables added to those from steps 1 and 2:

```sh
export EMBER_AGENTS_CONFIG_FILE=/etc/ember/agents.json
export EMBER_OPENCODE_PASSWORD_FILE=/etc/ember/opencode-password
export EMBER_OPENCODE_GATEWAY_KEY="$(sudo cat /etc/ember/opencode-gateway-key)"
export EMBER_OPENCODE_MODEL=claude-sonnet
./install.sh --check
./install.sh
```

`install.sh` adds `deploy/compose.opencode.yaml` when
`EMBER_OPENCODE_PASSWORD_FILE` is set; it needs the agents file and the model
gateway. Without `EMBER_ROUTER_SOCKET_DIR`, the router overlay is left out, so
a host can run session agents without the sandbox router. For a local trial on
a host without Kata, set `EMBER_OPENCODE_RUNTIME=runc`; do not do this where
agents handle untrusted input.

The worker:

- has no published port and is reachable only from the API, on the internal
  `opencode` network;
- reaches only LiteLLM, on the internal `opencode-models` network, with the
  provider settings in `deploy/opencode/opencode.json`;
- holds its virtual key in its environment. The agent can read it, which is
  why it is a virtual key with one model and a budget, never the master key;
- keeps `/workspace`, its home and `/tmp` on tmpfs. Every chat of this worker
  shares `/workspace`, and recreating the worker loses sessions and files.
  Project files are not mounted.

## Rotate or revoke a key

Create a new key with `new-key.py`, replace the file (or
`EMBER_OPENCODE_GATEWAY_KEY`), rerun `./install.sh`, then delete the old key
with LiteLLM's `POST /key/delete` (body `{"keys": ["sk-..."]}`, master key as
bearer token). Replacing `LITELLM_MASTER_KEY` needs `$boot up -d litellm`.

## Ansible

The Ansible `ember` role does not yet deploy these overlays: it would also
have to create the secret files above. Until it does, run steps 1–5 on the
host and add the `-f deploy/compose.models.yaml` (and
`-f deploy/compose.opencode.yaml`) files to the role's Compose file list by
hand, or use `install.sh` on that host.

## Data and backups

| Where | Holds | Back up? |
| --- | --- | --- |
| `/etc/ember/litellm/secrets` | Master key, database password, provider keys | As secrets |
| `litellm-db` volume | Virtual keys (hashed), budgets and spend | Yes |
| `ai` volume (`catalog.json`) | Offered models and agents; no keys | Yes |
| Worker tmpfs | Sessions and workspace files | Lost on recreate |
