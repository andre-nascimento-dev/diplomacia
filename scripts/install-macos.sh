#!/bin/sh
# Compila o Recado e instala neste Mac.
set -eu
cd "$(dirname "$0")/.."

PATH="$HOME/go/bin:/opt/homebrew/bin:$PATH"
wails build -clean -platform darwin/arm64
scripts/Instalar.command build/bin/Recado.app
