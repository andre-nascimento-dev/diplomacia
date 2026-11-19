#!/bin/sh
# Compila o DiplomacIA e instala neste Mac.
set -eu
cd "$(dirname "$0")/.."

PATH="$HOME/go/bin:/opt/homebrew/bin:$PATH"
scripts/changelog.sh > frontend/dist/changelog.json
wails build -clean -platform darwin/arm64
scripts/Instalar.command build/bin/DiplomacIA.app
