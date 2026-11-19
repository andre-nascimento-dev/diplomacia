package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

const modelFile = "ggml-large-v3-turbo-q5_0.bin"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "Library", "Application Support", "DiplomacIA")
	mediaDir := filepath.Join(dir, "media")
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		return err
	}
	db, err := openDB(filepath.Join(dir, "diplomacia.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	storePath := filepath.Join(dir, "whatsapp.db")

	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return runMCP(ctx, db, storePath)
	}
	if len(os.Args) > 1 && os.Args[1] == "logout" {
		return logout(context.Background(), storePath)
	}

	model := filepath.Join(dir, "models", modelFile)
	app := &App{db: db}
	return wails.Run(&options.App{
		Title:             "DiplomacIA",
		Width:             520,
		Height:            760,
		MinWidth:          360,
		MinHeight:         420,
		AssetServer:       &assetserver.Options{Assets: assets},
		HideWindowOnClose: true,
		OnStartup: func(ctx context.Context) {
			startTray(ctx)
			go transcribeLoop(ctx, db, model)
			wa, err := startWA(ctx, db, storePath, mediaDir)
			app.setWA(wa, err)
			if wa != nil {
				go wa.queueLoop(ctx)
			}
		},
		OnShutdown: func(context.Context) { app.shutdown() },
		Bind:       []any{app},
	})
}

type App struct {
	db  *sql.DB
	mu  sync.Mutex
	wa  *WA
	err error
}

func (a *App) setWA(wa *WA, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.wa, a.err = wa, err
}

func (a *App) shutdown() {
	if a.wa != nil {
		a.wa.cli.Disconnect()
	}
}

type Status struct {
	Connected bool   `json:"connected"`
	Detail    string `json:"detail"`
	QR        string `json:"qr"`
}

func (a *App) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case a.err != nil:
		return Status{Detail: "Falha ao iniciar o WhatsApp: " + a.err.Error()}
	case a.wa == nil:
		return Status{Detail: "Conectando…"}
	}
	detail, qr := a.wa.State()
	return Status{Connected: a.wa.cli.IsConnected() && a.wa.cli.IsLoggedIn(), Detail: detail, QR: qr}
}

func (a *App) Chats(query string) ([]Chat, error) { return listChats(a.db, query, 50, 0) }

func (a *App) DefaultTone() (string, error) { return defaultTone(a.db) }

func (a *App) SetDefaultTone(tone string) error { return setDefaultTone(a.db, tone) }

func (a *App) SetChatTone(jid, tone string) error { return setChatTone(a.db, jid, tone) }
