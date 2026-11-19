package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listChatsIn struct {
	Query  string `json:"query,omitempty" jsonschema:"parte do nome ou do número; vazio lista as conversas mais recentes"`
	Limit  int    `json:"limit,omitempty" jsonschema:"máximo de conversas, padrão 30"`
	Before int64  `json:"before,omitempty" jsonschema:"só conversas com a última mensagem anterior a este instante Unix em segundos; use o lastTs da última conversa da página para paginar"`
}

type readIn struct {
	Chat   string `json:"chat" jsonschema:"JID da conversa (de list_chats) ou número com DDI e DDD"`
	Limit  int    `json:"limit,omitempty" jsonschema:"máximo de mensagens, padrão 30"`
	Before int64  `json:"before,omitempty" jsonschema:"só mensagens anteriores a este instante Unix em segundos; use o ts da mensagem mais antiga lida para paginar"`
}

type olderIn struct {
	Chat  string `json:"chat" jsonschema:"JID da conversa (de list_chats) ou número com DDI e DDD"`
	Count int    `json:"count,omitempty" jsonschema:"quantas mensagens anteriores pedir ao celular, padrão 50, máximo 200"`
}

type searchIn struct {
	Query string `json:"query" jsonschema:"texto a procurar nas mensagens e nas transcrições dos áudios"`
	Limit int    `json:"limit,omitempty" jsonschema:"máximo de mensagens, padrão 30"`
}

type contactsIn struct {
	Query string `json:"query" jsonschema:"parte do nome ou do número do contato"`
}

type sendIn struct {
	Chat string `json:"chat" jsonschema:"JID da conversa ou número com DDI e DDD, por exemplo 5562999998888"`
	Text string `json:"text" jsonschema:"texto da mensagem"`
}

type mediaIn struct {
	Chat string `json:"chat" jsonschema:"JID da conversa (de list_chats) ou número com DDI e DDD"`
	ID   string `json:"id" jsonschema:"id da mensagem com arquivo, de read_messages ou search_messages"`
}

type sendFileIn struct {
	Chat    string `json:"chat" jsonschema:"JID da conversa ou número com DDI e DDD"`
	Path    string `json:"path" jsonschema:"caminho absoluto de um arquivo local; JPEG e PNG saem como foto, MP4 como vídeo e o resto como documento"`
	Caption string `json:"caption,omitempty" jsonschema:"legenda do arquivo"`
}

type sendAudioIn struct {
	Chat string `json:"chat" jsonschema:"JID da conversa ou número com DDI e DDD"`
	Path string `json:"path" jsonschema:"caminho absoluto de um arquivo de áudio local; o DiplomacIA converte para mensagem de voz"`
}

type toneIn struct {
	Chat string `json:"chat" jsonschema:"JID da conversa ou número com DDI e DDD"`
}

type setToneIn struct {
	Chat string `json:"chat,omitempty" jsonschema:"JID da conversa ou número com DDI e DDD; vazio grava o tom padrão de todas as conversas"`
	Tone string `json:"tone" jsonschema:"como escrever: formalidade, tamanho, emojis, tratamento; vazio apaga o tom"`
}

func runMCP(ctx context.Context, db *sql.DB, storePath string) error {
	s := mcp.NewServer(&mcp.Implementation{Name: "diplomacia", Version: "0.1.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{Name: "list_chats", Description: "Lista as conversas do WhatsApp gravadas pelo DiplomacIA, das mais recentes para as mais antigas. Pagine com before."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in listChatsIn) (*mcp.CallToolResult, any, error) {
			return jsonResult(listChats(db, in.Query, limitOr(in.Limit), in.Before))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "read_messages", Description: "Lê as últimas mensagens de uma conversa. Cada áudio vem com a transcrição no campo transcript. Uma mensagem com arquivo traz kind (image, video, audio, document, sticker), mime e fileName; leia o arquivo com get_media."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, any, error) {
			chat, err := normalizeChat(in.Chat)
			if err != nil {
				return nil, nil, err
			}
			return jsonResult(recentMessages(db, chat, limitOr(in.Limit), in.Before))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "get_media", Description: "Baixa e devolve o arquivo de uma mensagem: a foto como imagem, o vídeo como quadros e transcrição, o texto (md, txt, csv, json) como conteúdo, e o caminho local de todo arquivo, por exemplo um PDF. O app DiplomacIA precisa estar aberto para baixar um arquivo que ainda não está no Mac."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mediaIn) (*mcp.CallToolResult, any, error) {
			chat, err := normalizeChat(in.Chat)
			if err != nil {
				return nil, nil, err
			}
			return getMedia(ctx, db, chat, in.ID)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "load_older_messages", Description: "Pede ao celular as mensagens anteriores à mais antiga que o DiplomacIA tem de uma conversa, e grava e transcreve os áudios que chegam. O celular precisa estar online e o app DiplomacIA aberto."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in olderIn) (*mcp.CallToolResult, any, error) {
			chat, err := normalizeChat(in.Chat)
			if err != nil {
				return nil, nil, err
			}
			count := in.Count
			if count <= 0 || count > 200 {
				count = 50
			}
			return loadOlder(ctx, db, chat, count)
		})

	mcp.AddTool(s, &mcp.Tool{Name: "search_messages", Description: "Procura um texto em todas as mensagens e nas transcrições dos áudios."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
			if strings.TrimSpace(in.Query) == "" {
				return nil, nil, fmt.Errorf("informe o texto da busca")
			}
			return jsonResult(searchMessages(db, in.Query, limitOr(in.Limit)))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "list_contacts", Description: "Procura contatos da agenda do WhatsApp pelo nome ou pelo número."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in contactsIn) (*mcp.CallToolResult, any, error) {
			return jsonResult(findContacts(ctx, storePath, in.Query))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "get_tone", Description: "Devolve o tom de escrita para uma conversa: o tom da conversa, ou o tom padrão quando a conversa não tem um. Chame antes de redigir uma mensagem e siga o tom."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in toneIn) (*mcp.CallToolResult, any, error) {
			chat, err := normalizeChat(in.Chat)
			if err != nil {
				return nil, nil, err
			}
			return jsonResult(chatTone(db, chat))
		})

	mcp.AddTool(s, &mcp.Tool{Name: "set_tone", Description: "Grava o tom de escrita de uma conversa, ou o tom padrão quando chat fica vazio. Use só quando a pessoa pedir."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in setToneIn) (*mcp.CallToolResult, any, error) {
			if in.Chat == "" {
				if err := setDefaultTone(db, in.Tone); err != nil {
					return nil, nil, err
				}
				return textResult("Tom padrão gravado.")
			}
			chat, err := normalizeChat(in.Chat)
			if err != nil {
				return nil, nil, err
			}
			if err := setChatTone(db, chat, in.Tone); err != nil {
				return nil, nil, err
			}
			return textResult("Tom da conversa gravado.")
		})

	mcp.AddTool(s, &mcp.Tool{Name: "send_message", Description: "Envia uma mensagem de texto pelo WhatsApp. Antes de redigir, leia o tom da conversa com get_tone. O app DiplomacIA precisa estar aberto."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in sendIn) (*mcp.CallToolResult, any, error) {
			if strings.TrimSpace(in.Text) == "" {
				return nil, nil, fmt.Errorf("a mensagem está vazia")
			}
			return queueAndWait(ctx, db, Outgoing{Chat: in.Chat, Text: in.Text})
		})

	mcp.AddTool(s, &mcp.Tool{Name: "send_audio", Description: "Envia um arquivo de áudio como mensagem de voz pelo WhatsApp. O app DiplomacIA precisa estar aberto."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in sendAudioIn) (*mcp.CallToolResult, any, error) {
			if !filepath.IsAbs(in.Path) || !fileExists(in.Path) {
				return nil, nil, fmt.Errorf("o arquivo %q não existe; use um caminho absoluto", in.Path)
			}
			return queueAndWait(ctx, db, Outgoing{Chat: in.Chat, Audio: in.Path})
		})

	mcp.AddTool(s, &mcp.Tool{Name: "send_file", Description: "Envia um arquivo pelo WhatsApp: foto, vídeo, PDF ou qualquer documento. O app DiplomacIA precisa estar aberto."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in sendFileIn) (*mcp.CallToolResult, any, error) {
			if !filepath.IsAbs(in.Path) || !fileExists(in.Path) {
				return nil, nil, fmt.Errorf("o arquivo %q não existe; use um caminho absoluto", in.Path)
			}
			return queueAndWait(ctx, db, Outgoing{Chat: in.Chat, Text: in.Caption, File: in.Path})
		})

	return s.Run(ctx, &mcp.StdioTransport{})
}

func queueAndWait(ctx context.Context, db *sql.DB, o Outgoing) (*mcp.CallToolResult, any, error) {
	jid, err := normalizeChat(o.Chat)
	if err != nil {
		return nil, nil, err
	}
	o.Chat = jid
	id, err := enqueue(db, o, time.Now().Unix())
	if err != nil {
		return nil, nil, err
	}
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		status, errText, err := outboxStatus(db, id)
		if err != nil {
			return nil, nil, err
		}
		switch status {
		case "sent":
			return textResult(fmt.Sprintf("Mensagem enviada para %s.", jid))
		case "failed":
			return nil, nil, fmt.Errorf("o envio falhou: %s", errText)
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return textResult(fmt.Sprintf("O envio %d aguarda o app DiplomacIA. Abra o app: o envio sai quando ele conectar.", id))
}

func getMedia(ctx context.Context, db *sql.DB, chat, id string) (*mcp.CallToolResult, any, error) {
	m, err := findMessage(db, chat, id)
	if err != nil {
		return nil, nil, err
	}
	if m.Kind == "" {
		return nil, nil, fmt.Errorf("a mensagem %s não tem arquivo", id)
	}
	if _, statErr := os.Stat(m.File); m.File == "" || statErr != nil {
		if len(m.Media) == 0 {
			return nil, nil, fmt.Errorf("a mensagem %s chegou antes de o DiplomacIA guardar arquivos; peça para reenviar o arquivo", id)
		}
		if err := requestDownload(db, chat, id); err != nil {
			return nil, nil, err
		}
		m.Status = "download"
		for deadline := time.Now().Add(3 * time.Minute); m.Status == "download"; {
			if time.Now().After(deadline) {
				return textResult("O download aguarda o app DiplomacIA. Abra o app e chame get_media de novo.")
			}
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(time.Second):
			}
			if m, err = findMessage(db, chat, id); err != nil {
				return nil, nil, err
			}
		}
		if m.Status == "failed" {
			return nil, nil, fmt.Errorf("%s", m.Transcript)
		}
	}
	return mediaResult(ctx, m)
}

func mediaResult(ctx context.Context, m Message) (*mcp.CallToolResult, any, error) {
	info, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(m.File)
	if err != nil {
		return nil, nil, err
	}
	content := []mcp.Content{&mcp.TextContent{Text: string(info)}}
	base, _, _ := mime.ParseMediaType(m.Mime)
	switch {
	case slices.Contains([]string{"image/jpeg", "image/png", "image/webp", "image/gif"}, base) && len(data) <= 5<<20:
		content = append(content, &mcp.ImageContent{Data: data, MIMEType: base})
	case m.Kind == "video":
		frames, err := videoFrames(ctx, m.File, m.Seconds)
		if err != nil {
			return nil, nil, err
		}
		content = append(content, frames...)
	case m.Kind == "document" && len(data) <= 200<<10 && utf8.Valid(data):
		content = append(content, &mcp.TextContent{Text: string(data)})
	}
	return &mcp.CallToolResult{Content: content}, nil, nil
}

func videoFrames(ctx context.Context, path string, seconds int) ([]mcp.Content, error) {
	dir, err := os.MkdirTemp("", "diplomacia-frames-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	step := max(seconds/4, 1)
	if out, err := exec.CommandContext(ctx, tool("ffmpeg"), "-v", "error", "-i", path, "-vf", fmt.Sprintf("fps=1/%d,scale='min(1024,iw)':-2", step),
		"-frames:v", "4", filepath.Join(dir, "%d.jpg")).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, lastLine(out))
	}
	frames := []mcp.Content{}
	for i := 1; i <= 4; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%d.jpg", i))); err == nil {
			frames = append(frames, &mcp.ImageContent{Data: data, MIMEType: "image/jpeg"})
		}
	}
	return frames, nil
}

func loadOlder(ctx context.Context, db *sql.DB, chat string, count int) (*mcp.CallToolResult, any, error) {
	oldest, err := oldestMessage(db, chat)
	if err != nil {
		return nil, nil, err
	}
	id, err := requestHistory(db, chat, count, time.Now().Unix())
	if err != nil {
		return nil, nil, err
	}
	got, stable := 0, 0
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline) && stable < 4; {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(time.Second):
		}
		status, errText, err := historyRequestStatus(db, id)
		if err != nil {
			return nil, nil, err
		}
		if status == "failed" {
			return nil, nil, fmt.Errorf("o pedido ao celular falhou: %s", errText)
		}
		n, err := countBefore(db, chat, oldest.TS)
		if err != nil {
			return nil, nil, err
		}
		if n > 0 && n == got {
			stable++
		} else {
			got, stable = n, 0
		}
	}
	if status, _, _ := historyRequestStatus(db, id); status == "waiting" {
		return textResult("O pedido aguarda o app DiplomacIA. Abra o app e tente de novo.")
	}
	if got == 0 {
		return textResult(fmt.Sprintf("O celular não mandou mensagens anteriores a %s em 90 segundos. A conversa pode não ter mensagens mais antigas, ou o celular está offline.",
			time.Unix(oldest.TS, 0).Format("02/01/2006 15:04")))
	}
	return textResult(fmt.Sprintf("Chegaram %d mensagens anteriores a %s. Leia com read_messages e before = %d. Os áudios novos ficam com status pending até a transcrição terminar.",
		got, time.Unix(oldest.TS, 0).Format("02/01/2006 15:04"), oldest.TS))
}

type Contact struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

func findContacts(ctx context.Context, storePath, query string) ([]Contact, error) {
	db, err := sql.Open("sqlite3", "file:"+storePath+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	like := "%" + query + "%"
	rows, err := db.QueryContext(ctx, `SELECT their_jid, COALESCE(NULLIF(full_name, ''), NULLIF(first_name, ''), NULLIF(push_name, ''), business_name, '')
		FROM whatsmeow_contacts WHERE full_name LIKE ? OR first_name LIKE ? OR push_name LIKE ? OR their_jid LIKE ?
		ORDER BY full_name LIMIT 30`, like, like, like, like)
	if err != nil {
		return nil, fmt.Errorf("a agenda ainda não existe: abra o DiplomacIA e vincule o celular (%w)", err)
	}
	defer rows.Close()
	out := []Contact{}
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.JID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func limitOr(n int) int {
	if n <= 0 || n > 200 {
		return 30
	}
	return n
}

func jsonResult(v any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return nil, nil, err
	}
	return textResult(string(b))
}

func textResult(s string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}, nil, nil
}
