package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func transcribeLoop(ctx context.Context, db *sql.DB, model string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		if !fileExists(model) {
			continue
		}
		var chat, id, file string
		err := db.QueryRow(`SELECT chat, id, file FROM messages WHERE status = 'pending' ORDER BY ts DESC LIMIT 1`).Scan(&chat, &id, &file)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "transcribe queue:", err)
			continue
		}
		text, err := transcribe(ctx, model, file)
		status := "done"
		if err != nil {
			status, text = "failed", err.Error()
		}
		if _, err := db.Exec(`UPDATE messages SET transcript = ?, status = ? WHERE chat = ? AND id = ?`, text, status, chat, id); err != nil {
			fmt.Fprintln(os.Stderr, "transcribe save:", err)
		}
	}
}

func transcribe(ctx context.Context, model, file string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	wav, err := os.CreateTemp("", "recado-*.wav")
	if err != nil {
		return "", err
	}
	wav.Close()
	defer os.Remove(wav.Name())
	if out, err := exec.CommandContext(ctx, tool("ffmpeg"), "-y", "-i", file, "-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav.Name()).CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg: %v: %s", err, lastLine(out))
	}
	out, err := exec.CommandContext(ctx, tool("whisper-cli"), "-m", model, "-l", "pt", "-nt", "-np", "-f", wav.Name()).Output()
	if err != nil {
		return "", fmt.Errorf("whisper-cli: %v", err)
	}
	return strings.Join(strings.Fields(string(out)), " "), nil
}
