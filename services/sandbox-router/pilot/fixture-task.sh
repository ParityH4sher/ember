#!/bin/sh
# Deterministic acceptance fixture for the two files documented in README.md.
set -eu
cat >/dev/null
printf 'def add(a, b): return a + b\n' > calc.py
python3 -B -m unittest test_calc
