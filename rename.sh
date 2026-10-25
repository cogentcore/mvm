#!/bin/sh
# rename.sh renames the module of this fork from github.com/mvm-sh/mvm to
# github.com/cogentcore/mvm, which the cogentcore branch publishes: the
# goscript branch, and main, keep the upstream path so that their changes stay
# clean patches against upstream. It is idempotent: after merging goscript
# into cogentcore (resolving any conflicts in favor of goscript), run it to
# rename the paths in new code, then commit.
set -e
cd "$(dirname "$0")"
if [ -d gsoverride/github.com/mvm-sh/mvm ]; then
	mkdir -p gsoverride/github.com/cogentcore
	git mv gsoverride/github.com/mvm-sh/mvm gsoverride/github.com/cogentcore/mvm
	rmdir gsoverride/github.com/mvm-sh 2>/dev/null || true
fi
git grep -lI 'github\.com/mvm-sh/mvm' -- . ':!rename.sh' | while read -r f; do
	sed -i.bak 's#github\.com/mvm-sh/mvm#github.com/cogentcore/mvm#g' "$f"
	/bin/rm -f "$f.bak"
done
gofmt -l . | xargs -r gofmt -w
