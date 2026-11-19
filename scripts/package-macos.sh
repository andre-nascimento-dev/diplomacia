#!/bin/sh
# Compila o Recado e gera dist/Recado.zip (app + instalador) para enviar a outras pessoas.
set -eu
cd "$(dirname "$0")/.."

PATH="$HOME/go/bin:/opt/homebrew/bin:$PATH"
wails build -clean -trimpath -platform darwin/arm64

rm -rf dist
mkdir -p dist/Recado
cp -R scripts/Instalar.command "scripts/Como usar.txt" dist/Recado/
# App oculto: aberto ou arrastado direto do zip, o macOS diz "danificado" (sem Developer ID). Só o instalador o usa.
cp -R build/bin/Recado.app dist/Recado/.Recado.app
ditto -c -k --keepParent dist/Recado dist/Recado.zip
echo "Pacote em dist/Recado.zip"
