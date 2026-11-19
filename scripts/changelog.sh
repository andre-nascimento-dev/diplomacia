#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

esc() { sed 's/\\/\\\\/g; s/"/\\"/g'; }

version() {
  printf '{"version":"%s","date":"%s","notes":[' "$1" "$2"
  git log --no-merges --format=%s "$3" | esc | sed 's/.*/"&"/' | paste -sd, - | tr -d '\n'
  printf ']}'
}

tags=$(git tag -l 'v*' --sort=v:refname)
last=$(printf '%s\n' "$tags" | tail -n1)

entries=""
prev=""
for t in $tags; do
  entries="$(version "${t#v}" "$(git log -1 --format=%cs "$t")" "${prev:+$prev..}$t")${entries:+,$entries}"
  prev=$t
done

if [ -n "$last" ] && [ "$(git rev-parse HEAD)" = "$(git rev-parse "$last^{commit}")" ]; then
  current=${last#v}
else
  current=0.1.0
  [ -n "$last" ] && current=$(printf '%s' "${last#v}" | awk -F. '{print $1"."$2"."$3+1}')
  entries="$(version "$current" "$(date +%F)" "${last:+$last..}HEAD")${entries:+,$entries}"
fi

printf '{"current":"%s","versions":[%s]}\n' "$current" "$entries"
