import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from submit import snapshot


class SnapshotTest(unittest.TestCase):
    def test_only_selected_committed_regular_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "source").write_text("committed")
            (root / "link").symlink_to("/etc/passwd")
            (root / "private").write_text("not selected")
            def git(*args):
                subprocess.run(["git", *args], cwd=root, check=True)
            git("init", "-q")
            git("add", ".")
            git("-c", "user.name=fixture", "-c", "user.email=test@example.test", "commit", "-qm", "fixture")
            (root / "source").write_text("dirty")
            data = snapshot(root, ["source"], "task")
            self.assertEqual(data["files"], {"source": {"data": "Y29tbWl0dGVk", "executable": False}})
            for invalid in ["link", "../outside", ".git/config", os.fspath(root / "source"), "*"]:
                with self.assertRaises(ValueError):
                    snapshot(root, [invalid], "task")
