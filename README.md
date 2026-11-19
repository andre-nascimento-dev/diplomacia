# DiplomacIA

WhatsApp no Claude: lê e envia mensagens, áudios e arquivos, no tom certo para cada conversa.

- O app grava as conversas neste Mac e transcreve os áudios.
- O Claude lê e envia mensagens, fotos, vídeos e documentos pelo servidor MCP `diplomacia`.
- Cada conversa tem um tom próprio. Sem tom próprio, a conversa usa o tom padrão.
- O botão "Change logs" mostra as novidades de cada versão.

## Instalar

Siga [scripts/Como usar.txt](scripts/Como%20usar.txt).

## Atalhos no Claude Code

| Atalho | Faz |
|---|---|
| `/mcp__diplomacia__responder` | Redige a resposta no tom da conversa e envia depois da confirmação. |
| `/mcp__diplomacia__resumir` | Resume fatos, decisões, pedidos e prazos de uma conversa. |
| `/mcp__diplomacia__pendentes` | Lista as conversas que esperam uma resposta. |
| `/mcp__diplomacia__tom` | Mostra ou grava o tom de uma conversa ou o tom padrão. |

## Tools do MCP

`list_chats`, `read_messages`, `get_media`, `load_older_messages`, `search_messages`, `list_contacts`,
`get_tone`, `set_tone`, `send_message`, `send_audio` e `send_file`.

## Desenvolver

Requer Go, o Wails v2 o `ffmpeg` e o `whisper-cpp` do Homebrew.

```bash
go test ./...
scripts/install-macos.sh
```

- `scripts/install-macos.sh` compila e instala neste Mac.
- `scripts/package-macos.sh` gera `dist/DiplomacIA.zip` para outras pessoas.
- `scripts/release.sh` roda na `main` depois do merge. Ele cria a tag `vX.Y.Z`, publica o zip no GitHub e instala.

## Versões e changelog

`scripts/changelog.sh` gera o changelog no build, a partir das tags `v*` e dos commits entre elas.
Os commits depois da última tag formam a próxima versão.
Use o prefixo do commit para o tipo da nota: `feat` é Novidade, `fix` é Correção e `perf` é Melhoria.
