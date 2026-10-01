#!/bin/sh
# usage: tools/commit.sh "message"
cd "$(dirname "$0")/.." || exit 1
git add -A && git -c user.name=librarian-agent -c user.email=billy+lib@babalou2 commit -q -m "$1" && git push -q -u origin librarian/sandbox 2>&1 | tail -2
git log --oneline -1
