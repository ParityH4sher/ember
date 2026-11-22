#!/usr/bin/env python3
"""Image-side /ember-agent wrapper. All filesystem and tool activity is in the guest."""
import base64
import json
import os
from pathlib import Path
import subprocess
import sys


def git(*args):
    return subprocess.check_output(["git", *args], stderr=sys.stderr)


def stage(files):
    if not isinstance(files, dict) or not 1 <= len(files) <= 64:
        raise ValueError("invalid files")
    remaining = 512 * 1024
    for name, blob in files.items():
        if any(p in ("", ".", "..", ".git") for p in name.lower().split("/")):
            raise ValueError("invalid snapshot path")
        data = base64.b64decode(blob["data"], validate=True)
        remaining -= len(data)
        if remaining < 0:
            raise ValueError("snapshot too large")
        path = Path(name)
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("xb") as f:
            f.write(data)
        path.chmod(0o755 if blob["executable"] else 0o644)


def main():
    task = json.load(sys.stdin)
    if task["version"] != 1:
        raise ValueError("unknown snapshot format")
    # The source directory is new and separate from the guest's HOME.
    Path("project").mkdir()
    os.chdir("project")
    stage(task["files"])
    git("init", "-q")
    git("add", "-f", "--all")
    git("-c", "user.name=Ember", "-c", "user.email=pilot@localhost",
        "commit", "-qm", "Snapshot")
    # Fixed, image-owned adapter. Its stdout is the agent/test report; the task
    # goes to stdin and is never interpreted as a shell command by the wrapper.
    with open("/tmp/agent-report", "w+b") as report:
        status = subprocess.run(["/usr/local/bin/ember-task"], input=task["task"].encode(),
                                stdout=report, stderr=subprocess.STDOUT).returncode
        report.seek(0)
        text = report.read(8192).decode(errors="replace")
    git("add", "-N", "--all")
    patch = git("diff", "--no-ext-diff", "--no-textconv", "--binary", "HEAD").decode()
    print(json.dumps({"patch": patch, "report": f"Adapter exit: {status}\n{text}"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
