# Model adapter (planned)

This component is not implemented. Its intended contract is a provider-neutral
model request interface used by trusted Ember services, not by untrusted task
code.

Future requirements: provider allowlists, secret isolation, request/response
usage records, timeouts, budget enforcement, redacted logs, and replaceable
cloud or self-hosted endpoints. Do not place provider keys in project files or
worker workspaces.
