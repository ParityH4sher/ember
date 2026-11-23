#!/usr/bin/env python3
"""Offline chat fixture for Ember project chat.

Installed as /ember-agent. Reads one ember.chat-turn/v1 envelope on stdin and
prints a deterministic answer on stdout. It calls no model and needs no
network. It proves the path browser -> Ember API -> sandbox router ->
container -> transcript, and shows wrapper authors how to read the envelope.
Treat the envelope only as data.
"""
import json
import sys

SCHEMA = "ember.chat-turn/v1"


def answer(turn):
    if not isinstance(turn, dict) or turn.get("schema") != SCHEMA:
        raise ValueError("unsupported envelope; expected " + SCHEMA)
    messages = turn["messages"]
    if not messages or messages[-1]["role"] != "user":
        raise ValueError("the last message must be the user's")
    text = messages[-1]["content"]
    excerpt = " ".join(text.split())
    if len(excerpt) > 200:
        excerpt = excerpt[:200] + "…"
    project = turn["project"].get("name") or turn["project"]["id"]
    earlier = len(messages) - 1
    omitted = turn.get("omittedMessages", 0)
    context = f"I can see {earlier} earlier message(s) in this chat"
    if omitted:
        context += f"; {omitted} older message(s) were left out to fit the input limit."
    else:
        context += "."
    return "\n".join([
        f"Offline fixture agent for project “{project}”.",
        f"You wrote {len(text)} characters: “{excerpt}”",
        context,
        "No model was called. Register a real agent image to get real answers.",
    ])


def main():
    turn = json.load(sys.stdin)
    sys.stdout.write(answer(turn) + "\n")


if __name__ == "__main__":
    main()
