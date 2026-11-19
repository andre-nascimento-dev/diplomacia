#!/bin/sh
# Instala o Recado.app em /Applications, as ferramentas de transcrição e o modelo na primeira vez.
# Sem argumento, usa o .Recado.app oculto ao lado deste arquivo (o pacote enviado a outras pessoas).
set -eu
src=${1:-"$(dirname "$0")/.Recado.app"}
PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"

[ "$(uname -m)" = arm64 ] || { echo "ERRO: o Recado só funciona em Mac com chip Apple (M1 ou mais novo)."; exit 1; }
# "Operation not permitted" aqui é falta de permissão do Terminal para ler a pasta (Ajustes > Privacidade e Segurança > Arquivos e Pastas).
[ -r "$src/Contents/MacOS/Recado" ] || { echo "ERRO: não consigo ler $src. Dê ao Terminal acesso à pasta em Ajustes > Privacidade e Segurança > Arquivos e Pastas."; exit 1; }

echo "1/4 Instalando o Recado em /Applications..."
app=/Applications/Recado.app
# Trocar o app com ele aberto deixa a janela em branco: o WebKit recusa o binário antigo, que não bate com a assinatura nova.
osascript -e 'if application id "com.wails.recado" is running then tell application id "com.wails.recado" to quit' >/dev/null 2>&1 || true
rm -rf "$app"
ditto "$src" "$app"
# O app chega de fora sem assinatura da Apple; sem isto o Gatekeeper bloqueia a abertura.
xattr -dr com.apple.quarantine "$app" 2>/dev/null || true
codesign --force --deep --sign - "$app"
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$app" || true

echo "2/4 Instalando ffmpeg e whisper-cpp..."
if command -v brew >/dev/null; then
  brew list ffmpeg whisper-cpp >/dev/null 2>&1 || brew install ffmpeg whisper-cpp || echo "AVISO: o brew falhou. O Recado não transcreve áudios até você executar este instalador de novo."
else
  echo "AVISO: sem Homebrew, o Recado não transcreve áudios. Instale o Homebrew (https://brew.sh) e execute este instalador de novo."
fi

echo "3/4 Baixando o modelo de transcrição (550 MB, só na primeira vez)..."
model="$HOME/Library/Application Support/Recado/models/ggml-large-v3-turbo-q5_0.bin"
if [ ! -f "$model" ]; then
  mkdir -p "$(dirname "$model")"
  curl -fL -C - --retry 3 --progress-bar -o "$model.part" https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin
  mv "$model.part" "$model"
fi

echo "4/4 Ligando o Recado ao Claude Code..."
if command -v claude >/dev/null; then
  claude mcp get recado >/dev/null 2>&1 || claude mcp add -s user recado -- "$app/Contents/MacOS/Recado" mcp || echo "AVISO: não consegui ligar o Recado ao Claude Code."
fi
open "$app"
echo "Instalado em $app"
