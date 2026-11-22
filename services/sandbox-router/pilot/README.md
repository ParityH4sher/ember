# Git snapshot pilot

On a Kata-qualified Linux host, use the [router setup](../README.md) and an
operator-owned Git checkout. Run the following from `services/sandbox-router`.
The fixture is an offline stand-in for a CLI: it fixes an intentionally broken
addition function and runs its unit test. It does not need a model credential.

```sh
docker --host unix:///var/run/docker.sock build -t ember-snapshot-fixture:dev pilot
docker --host unix:///var/run/docker.sock image inspect --format '{{.Id}}' ember-snapshot-fixture:dev
```

Register that Docker image ID in the router policy under `coding-fixture` with
`"profile":"isolated"`, and restart the router. Set `SOCKET` to its private
socket path. Create a disposable two-file repository:

```sh
work=$(mktemp -d)
git init -q "$work/source"
printf 'def add(a, b): return a - b\n' > "$work/source/calc.py"
printf 'import unittest, calc\nclass TestAdd(unittest.TestCase):\n    def test_add(self): self.assertEqual(calc.add(2, 3), 5)\n' > "$work/source/test_calc.py"
git -C "$work/source" add calc.py test_calc.py
git -C "$work/source" -c user.name=Fixture -c user.email=fixture@localhost commit -qm fixture
python3 pilot/submit.py --socket "$SOCKET" --agent coding-fixture \
  --repo "$work/source" --task 'Fix the failing addition test' \
  --out "$work/result" calc.py test_calc.py
cat "$work/result/report.txt"
git -C "$work/source" apply --check "$work/result/result.patch"
git -C "$work/source" status --short
```

Expect a report ending in `OK`, a patch changing `-` to `+`, and an unchanged
source checkout. The report is guest-supplied evidence. Inspect the patch
before choosing to apply it. `run.json` records the router run ID, commit and
selected files. For a repository hosted by Gitea, clone it on the operator side
first; guest access to the forge is unnecessary.

The submitter sends only explicitly named regular files from the resolved HEAD
commit (up to 64 files, 512 KiB total). The working tree, Git history, symlinks,
and submodules are not transferred. The private `/v1/snapshot-runs` endpoint
enforces `isolated` and a 1 MiB JSON request limit. The guest builds a temporary
Git repository and returns a patch and report through the 64 KiB stdout limit.
Source files, new files and test reports should be small enough for this limit.

To use another agent, package Python 3, Git and the agent's dependencies in an
image, copy `entrypoint.py` to `/ember-agent`, and provide an executable
`/usr/local/bin/ember-task` that reads the objective from stdin, edits files in
its current directory, runs tests, and prints its report. A nonzero adapter
exit is recorded in the report so a failed test can be reviewed with its patch.
The wrapper runs entirely inside the guest. Both profiles are offline; a live
hosted-model CLI still needs a separately scoped model broker. Socket access is
operator-only and the synchronous request must remain connected for the run.
