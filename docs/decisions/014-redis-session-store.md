# ADR 014: Redis session store

Status: Accepted  
Date: 2026-11-23  
Scope: Where the API keeps sign-in sessions and pending sign-in attempts, and
how the Compose and Ansible deployments run that store. Applies
[ADR 005](005-go-standard-library-first.md) to the Redis client.

**Problem.** The API kept [OpenID Connect](../authentication.md) sessions and
pending sign-in attempts (state, nonce, PKCE verifier) in process memory.
Every API restart, including each deployment, signed everyone out and broke
sign-ins in progress. A second API replica could not see the first one's
sessions. The operator asked for Redis.

**Decision.**

- The auth package has a small session-store interface (add, look up and
  remove a session; add and take a pending attempt). The in-memory store stays
  the default. `EMBER_REDIS_ADDR` (`host:port`) selects a Redis store, and
  `EMBER_REDIS_PASSWORD_FILE` optionally names its password, read like the OIDC
  client secret. A password file without an address, and Redis without sign-in,
  are startup errors.
- Keys are `ember:session:<hex SHA-256 of the cookie>` and
  `ember:pending:<hex SHA-256 of the state>`, so a copy of the data cannot be
  replayed as a cookie or used to finish someone's sign-in. Values are JSON:
  user, role, ID token and expiry for sessions; nonce, verifier, return path
  and expiry for attempts. Each key expires with its session or attempt
  (`SET … PX`). A pending attempt is read and deleted with `GETDEL`, so it is
  single-use across replicas.
- The store fails closed. When Redis does not answer, sign-in, sign-out and
  every request that needs a session get `503 session_store_unavailable`, like
  an identity provider outage. A Redis error never signs anyone in and never
  clears a cookie. `/healthz` stays up, and the API starts even while Redis is
  down.
- Compose runs the official `redis` 8 Alpine image, pinned by tag and
  multi-architecture digest, in `deploy/compose.redis.yaml`. `install.sh` and
  the Ansible `ember` role add it whenever sign-in is on, and generate its
  password. Redis listens only on an internal `sessions` network that the API
  alone joins; no port is published. It runs as its image user with a
  read-only root filesystem, no capabilities, `no-new-privileges`, and PID and
  memory limits. The password reaches Redis through a config file written to a
  tmpfs at start, not through the command line or environment. Data is kept
  with AOF (`appendfsync everysec`) in the `redis-data` volume.
  `maxmemory` with `volatile-ttl` evicts the entries closest to expiry first
  rather than refusing writes.

**Why Redis.** Sessions are small, expiring key-value records, which is what
Redis does natively: TTLs, atomic `GETDEL`, and persistence through restarts.
It is widely packaged, and one small container on the Raspberry Pi is enough.
PostgreSQL (already in the Authentik stack) would need a schema, migrations
and expiry sweeps, and would couple Ember to Authentik's database. A file
store would not serve several replicas.

**License.** Redis 8 is tri-licensed under RSALv2, SSPLv1 and AGPLv3. Ember
only runs the unmodified image as a separate service and talks to it over the
network. Operators who prefer an OSI license without copyleft can run
[Valkey](https://valkey.io), the BSD-licensed fork of Redis 7.2: the API uses
only commands Valkey implements (`AUTH`, `PING`, `SET` with `PX`, `GET`,
`DEL`, `GETDEL`). Change the image in the overlay; nothing else differs.

**Why a hand-written client.** ADR 005 asks for the standard library unless a
dependency saves substantial, risky work. The API needs six commands over
RESP2, which take about 300 lines with `net` and `bufio`: a bounded pool of
connections, dial and I/O deadlines, `AUTH` on connect, reply size limits, and
reconnection after errors. A client library would bring pipelining, cluster
and pub/sub support Ember does not use, plus its own update stream. The client
is tested against a fake RESP server in the Go tests.

**Consequences.**

- Sign-ins survive API restarts and redeployments. Several API replicas could
  share sessions, although Ember deploys one today.
- The `redis-data` volume holds ID tokens and user names, emails and groups.
  Protect it like the OIDC client secret. **Leave it out of backups**: sessions
  are disposable, and restoring an old or empty store only means everyone
  signs in again.
- Restarting the API no longer signs everyone out. To withdraw a role at once,
  clear the store (see [the sign-in guide](../authentication.md#session-store)).
- Redis is one more container (memory-limited to 192 MB) and one more
  image to update. Updating it means changing the tag and digest together.
- Redis is reached without TLS, so it must stay on a private network. The
  client supports neither Redis Cluster nor Sentinel.
- If Redis is down, signed-in Ember is paused until it returns.
