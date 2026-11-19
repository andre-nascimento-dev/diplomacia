#!/bin/sh
# Compila o DiplomacIA e gera dist/DiplomacIA.zip (app + instalador) para enviar a outras pessoas.
set -eu
cd "$(dirname "$0")/.."

PATH="$HOME/go/bin:/opt/homebrew/bin:$PATH"
scripts/changelog.sh > frontend/dist/changelog.json
wails build -clean -trimpath -platform darwin/arm64

rm -rf dist
mkdir -p dist/DiplomacIA
cp -R scripts/Instalar.command "scripts/Como usar.txt" dist/DiplomacIA/
# App oculto: aberto ou arrastado direto do zip, o macOS diz "danificado" (sem Developer ID). Só o instalador o usa.
cp -R build/bin/DiplomacIA.app dist/DiplomacIA/.DiplomacIA.app
ditto -c -k --keepParent dist/DiplomacIA dist/DiplomacIA.zip
echo "Pacote em dist/DiplomacIA.zip"
