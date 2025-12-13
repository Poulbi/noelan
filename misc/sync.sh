#!/bin/sh

set -eu

ScriptDirectory="$(dirname "$(readlink -f "$0")")"
cd "$ScriptDirectory"/..

rsync -arPv --exclude-from=.gitignore . keyfried.com:/srv/noel/
