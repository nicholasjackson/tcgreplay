#!/bin/sh
# usage: append_changelog.sh <entry-file>; appends entry to plan's ## Changelog (creating it at end if absent)
set -e
N=20261112091756-c5b6d1fd-launch-docs
T=.spektacular/tmp/$N
mkdir -p $T
spektacular plan file read $N plan > $T/plan_update.md
if ! grep -q '^## Changelog' $T/plan_update.md; then printf '\n## Changelog\n' >> $T/plan_update.md; fi
printf '\n' >> $T/plan_update.md
cat "$1" >> $T/plan_update.md
spektacular plan file write $N plan --from $T/plan_update.md
rm $T/plan_update.md
