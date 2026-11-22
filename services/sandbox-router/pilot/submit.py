"""Submit explicitly selected Git blobs to an isolated agent over the operator socket."""
import argparse
import base64
import http.client
import json
from pathlib import Path
import socket
import subprocess
import sys


def snapshot(repo, paths, task):
    def git(*args):
        return subprocess.check_output(["git", "-C", repo, *args])

    commit = git("rev-parse", "--verify", "HEAD^{commit}").decode().strip()
    files = {}
    remaining = 512 * 1024
    if not 1 <= len(paths) <= 64:
        raise ValueError("select 1–64 individual source files")
    for path in paths:
        if any(p in ("", ".", "..", ".git") for p in path.lower().split("/")):
            raise ValueError(f"invalid project-relative path: {path!r}")
        entry = git("--literal-pathspecs", "ls-tree", "-z", "--full-tree", commit, "--", path)
        meta, _, name = entry.rstrip(b"\0").partition(b"\t")
        if name != path.encode() or meta.split()[:2] not in ([b"100644", b"blob"], [b"100755", b"blob"]):
            raise ValueError(f"not a committed regular file: {path!r}")
        mode, _, oid = meta.split()
        size = int(git("cat-file", "-s", oid))
        remaining -= size
        if remaining < 0:
            raise ValueError("selected files exceed 512 KiB")
        files[path] = {"data": base64.b64encode(git("cat-file", "blob", oid)).decode(),
                       "executable": mode == b"100755"}
    return {"version": 1, "commit": commit, "task": task, "files": files}


class LocalHTTP(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=85)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for option in ("socket", "agent", "repo", "task", "out"):
        parser.add_argument("--" + option, required=True)
    parser.add_argument("paths", nargs="+", help="literal repository-root-relative paths in HEAD")
    args = parser.parse_args()
    payload = snapshot(args.repo, args.paths, args.task)
    request = json.dumps({"agent": args.agent, "input": json.dumps(payload)}).encode()
    if len(request) > 1 << 20:
        raise ValueError("snapshot request exceeds 1 MiB")
    # Reserve a new private output directory; never modify or apply to the source.
    out = Path(args.out)
    out.mkdir(mode=0o700)
    client = LocalHTTP(args.socket)
    try:
        client.request("POST", "/v1/snapshot-runs", request, {"Content-Type": "application/json"})
        response = client.getresponse()
        if response.status != 200:
            raise ValueError(f"router returned {response.status}: {response.read(512)!r}")
        run = json.loads(response.read(512 * 1024))
    finally:
        client.close()
    if run["profile"] != "isolated":
        raise ValueError("unexpected execution profile")
    result = json.loads(run["output"])
    if not isinstance(result["patch"], str) or not isinstance(result["report"], str):
        raise ValueError("expected text patch and report from agent")
    (out / "result.patch").write_bytes(result["patch"].encode())
    (out / "report.txt").write_bytes(result["report"].encode())
    (out / "run.json").write_text(json.dumps({"run_id": run["run_id"], "agent": args.agent,
        "profile": run["profile"], "commit": payload["commit"], "paths": args.paths}, indent=2))
    print(f"Review the patch and report in {out}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError, http.client.HTTPException) as exc:
        sys.exit(str(exc))
