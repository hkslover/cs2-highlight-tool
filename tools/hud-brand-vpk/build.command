#!/bin/bash
set -e
cd "$(dirname "$0")"

finish() {
    rc=$?
    if [ "$rc" -ne 0 ]; then
        echo "Build failed. See the error above."
    fi
    if [ -t 0 ]; then
        read -r -p "Press Enter to close..." || true
    fi
    exit "$rc"
}
trap finish EXIT

if [ -x .venv/bin/python ]; then
    python_bin=.venv/bin/python
elif python3 -c 'import PIL' >/dev/null 2>&1; then
    python_bin=python3
else
    python3 -m venv .venv
    python_bin=.venv/bin/python
fi
if ! "$python_bin" -c 'import PIL' >/dev/null 2>&1; then
    "$python_bin" -m pip install -r requirements.txt
fi
"$python_bin" build_vpk.py "$@"
