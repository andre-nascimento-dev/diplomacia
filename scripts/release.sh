#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

[ "$(git branch --show-current)" = main ] || { echo "Rode na main, depois do merge." >&2; exit 1; }
git pull -q --ff-only
version=$(scripts/changelog.sh | sed -n 's/^{"current":"\([^"]*\)".*/\1/p')
git tag -l "v$version" | grep -q . && { echo "A versão $version já existe." >&2; exit 1; }
last=$(git tag -l 'v*' --sort=v:refname | tail -n1)
notes=$(git log --no-merges --format='- %s' "${last:+$last..}HEAD")

git tag "v$version"
git push -q origin "v$version"
scripts/package-macos.sh
gh release create "v$version" dist/DiplomacIA.zip --title "DiplomacIA $version" --notes "$notes"
scripts/Instalar.command build/bin/DiplomacIA.app
