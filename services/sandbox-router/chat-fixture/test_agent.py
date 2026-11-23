import io
import json
import unittest
from contextlib import redirect_stdout
from unittest import mock

import agent


def turn(*contents, omitted=0, schema=agent.SCHEMA):
    roles = ["user", "agent"]
    return {
        "schema": schema,
        "project": {"id": "desk", "name": "Desk organizer"},
        "chat": {"id": "0123456789abcdef", "title": "Filament"},
        "messages": [{"role": roles[i % 2], "content": c} for i, c in enumerate(contents)],
        "omittedMessages": omitted,
    }


class AnswerTest(unittest.TestCase):
    def test_answers_the_last_user_message(self):
        reply = agent.answer(turn("Which filament?", "PETG.", "And  for\noutdoors?"))
        self.assertIn("“Desk organizer”", reply)
        self.assertIn("“And for outdoors?”", reply)
        self.assertIn("2 earlier message(s)", reply)
        self.assertNotIn("left out", reply)

    def test_reports_omitted_messages_and_shortens(self):
        reply = agent.answer(turn("x" * 300, omitted=4))
        self.assertIn("4 older message(s) were left out", reply)
        self.assertIn("x" * 200 + "…", reply)
        self.assertIn("300 characters", reply)

    def test_rejects_other_input(self):
        for bad in (turn("hi", schema="ember.chat-turn/v2"), turn("hi", "answer"), turn(), ["not", "a", "turn"]):
            with self.assertRaises((ValueError, KeyError, IndexError, AttributeError)):
                agent.answer(bad)

    def test_main_reads_stdin_and_writes_stdout(self):
        out = io.StringIO()
        with mock.patch("sys.stdin", io.StringIO(json.dumps(turn("hello")))), redirect_stdout(out):
            agent.main()
        self.assertTrue(out.getvalue().startswith("Offline fixture agent"))
        self.assertTrue(out.getvalue().endswith("\n"))


if __name__ == "__main__":
    unittest.main()
