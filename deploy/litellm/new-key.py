"""Create a LiteLLM virtual key and print only the key (docs/models.md).

Run inside the LiteLLM container, which can read the master key file:
  docker compose ... exec -T litellm python - ALIAS [MODELS] [MAX_BUDGET] < deploy/litellm/new-key.py

ALIAS names the key holder (for example ember-api or opencode-worker).
MODELS is a comma-separated list of gateway model names; empty allows all.
MAX_BUDGET is a spending limit in US dollars per 30 days; empty means none.
"""

import json
import sys
import urllib.error
import urllib.request

if not 2 <= len(sys.argv) <= 4:
    sys.exit(__doc__)
alias = sys.argv[1]
models = [m for m in (sys.argv[2] if len(sys.argv) > 2 else "").split(",") if m]
body = {"key_alias": alias, "models": models}
if len(sys.argv) > 3 and sys.argv[3]:
    body["max_budget"] = float(sys.argv[3])
    body["budget_duration"] = "30d"

with open("/run/secrets/litellm/LITELLM_MASTER_KEY") as f:
    master = f.readline().strip()
request = urllib.request.Request(
    "http://127.0.0.1:4000/key/generate",
    data=json.dumps(body).encode(),
    headers={"Authorization": "Bearer " + master, "Content-Type": "application/json"},
)
try:
    with urllib.request.urlopen(request, timeout=10) as response:
        print(json.load(response)["key"])
except urllib.error.HTTPError as e:
    # LiteLLM's error text explains the problem and holds no key.
    sys.exit(f"LiteLLM answered HTTP {e.code}: {e.read(2000).decode(errors='replace')}")
