package main

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const schema = `
CREATE TABLE IF NOT EXISTS chats (jid TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', last_ts INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS messages (
	chat TEXT NOT NULL, id TEXT NOT NULL, sender TEXT NOT NULL, sender_name TEXT NOT NULL,
	from_me INTEGER NOT NULL, ts INTEGER NOT NULL, text TEXT NOT NULL DEFAULT '',
	audio TEXT NOT NULL DEFAULT '', seconds INTEGER NOT NULL DEFAULT 0,
	transcript TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (chat, id)
);
CREATE INDEX IF NOT EXISTS messages_ts ON messages (chat, ts);
CREATE INDEX IF NOT EXISTS messages_status ON messages (status);
CREATE TABLE IF NOT EXISTS outbox (
	id INTEGER PRIMARY KEY, chat TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', audio TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'waiting', error TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS history_requests (
	id INTEGER PRIMARY KEY, chat TEXT NOT NULL, count INTEGER NOT NULL,
	status TEXT NOT NULL DEFAULT 'waiting', error TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL
);`

var migrations = []string{`
ALTER TABLE messages RENAME COLUMN audio TO file;
ALTER TABLE messages ADD COLUMN kind TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN mime TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN file_name TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN size INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN media BLOB;
UPDATE messages SET kind = 'audio', mime = 'audio/ogg' WHERE file <> '';
ALTER TABLE outbox ADD COLUMN file TEXT NOT NULL DEFAULT '';`, `
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
ALTER TABLE chats ADD COLUMN tone TEXT NOT NULL DEFAULT '';`}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for _, m := range migrations[min(version, len(migrations)):] {
		if _, err := tx.Exec(m); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations))); err != nil {
		return err
	}
	return tx.Commit()
}

type Message struct {
	Chat       string `json:"chat"`
	ChatName   string `json:"chatName"`
	ID         string `json:"id"`
	SenderName string `json:"senderName"`
	FromMe     bool   `json:"fromMe"`
	TS         int64  `json:"ts"`
	Text       string `json:"text"`
	Kind       string `json:"kind,omitempty"`
	Mime       string `json:"mime,omitempty"`
	FileName   string `json:"fileName,omitempty"`
	Size       int64  `json:"size,omitempty"`
	File       string `json:"file,omitempty"`
	Seconds    int    `json:"seconds"`
	Transcript string `json:"transcript"`
	Status     string `json:"status"`
	Media      []byte `json:"-"`
}

func saveMessage(db *sql.DB, m Message) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT OR IGNORE INTO messages (chat, id, sender, sender_name, from_me, ts, text, kind, mime, file_name, size, file, seconds, transcript, status, media)
		VALUES (?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, m.Chat, m.ID, m.SenderName, m.FromMe, m.TS, m.Text,
		m.Kind, m.Mime, m.FileName, m.Size, m.File, m.Seconds, m.Transcript, m.Status, m.Media)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO chats (jid, name, last_ts) VALUES (?, ?, ?)
		ON CONFLICT (jid) DO UPDATE SET last_ts = MAX(last_ts, excluded.last_ts),
		name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE chats.name END`, m.Chat, m.ChatName, m.TS)
	if err != nil {
		return err
	}
	return tx.Commit()
}

const messageColumns = `m.chat, c.name, m.id, m.sender_name, m.from_me, m.ts, m.text, m.kind, m.mime, m.file_name, m.size, m.file, m.seconds, m.transcript, m.status, m.media`

func queryMessages(db *sql.DB, where string, args ...any) ([]Message, error) {
	rows, err := db.Query(`SELECT `+messageColumns+` FROM messages m JOIN chats c ON c.jid = m.chat `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.Chat, &m.ChatName, &m.ID, &m.SenderName, &m.FromMe, &m.TS, &m.Text,
			&m.Kind, &m.Mime, &m.FileName, &m.Size, &m.File, &m.Seconds, &m.Transcript, &m.Status, &m.Media); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func recentMessages(db *sql.DB, chat string, limit int, before int64) ([]Message, error) {
	if before <= 0 {
		before = math.MaxInt64
	}
	if chat == "" {
		return queryMessages(db, `WHERE m.ts < ? ORDER BY m.ts DESC LIMIT ?`, before, limit)
	}
	return queryMessages(db, `WHERE m.chat = ? AND m.ts < ? ORDER BY m.ts DESC LIMIT ?`, chat, before, limit)
}

func oldestMessage(db *sql.DB, chat string) (Message, error) {
	found, err := queryMessages(db, `WHERE m.chat = ? ORDER BY m.ts, m.id LIMIT 1`, chat)
	if err != nil {
		return Message{}, err
	}
	if len(found) == 0 {
		return Message{}, fmt.Errorf("o DiplomacIA não tem mensagem da conversa %s; o celular só manda o histórico anterior a uma mensagem conhecida", chat)
	}
	return found[0], nil
}

func findMessage(db *sql.DB, chat, id string) (Message, error) {
	found, err := queryMessages(db, `WHERE m.chat = ? AND m.id = ?`, chat, id)
	if err != nil {
		return Message{}, err
	}
	if len(found) == 0 {
		return Message{}, fmt.Errorf("a mensagem %s não existe na conversa %s", id, chat)
	}
	return found[0], nil
}

func requestDownload(db *sql.DB, chat, id string) error {
	_, err := db.Exec(`UPDATE messages SET status = 'download' WHERE chat = ? AND id = ?`, chat, id)
	return err
}

func nextDownload(db *sql.DB) (Message, bool, error) {
	found, err := queryMessages(db, `WHERE m.status = 'download' ORDER BY m.ts DESC LIMIT 1`)
	if err != nil || len(found) == 0 {
		return Message{}, false, err
	}
	return found[0], true, nil
}

func afterDownload(kind string, err error) (status, transcript string) {
	switch {
	case err != nil:
		return "failed", "o arquivo não baixou: " + err.Error()
	case kind == "audio" || kind == "video":
		return "pending", ""
	}
	return "done", ""
}

func finishDownload(db *sql.DB, m Message, file string, downloadErr error) error {
	status, transcript := afterDownload(m.Kind, downloadErr)
	_, err := db.Exec(`UPDATE messages SET file = ?, status = ?, transcript = ? WHERE chat = ? AND id = ?`,
		file, status, transcript, m.Chat, m.ID)
	return err
}

func countBefore(db *sql.DB, chat string, ts int64) (n int, err error) {
	err = db.QueryRow(`SELECT count(*) FROM messages WHERE chat = ? AND ts < ?`, chat, ts).Scan(&n)
	return
}

type HistoryRequest struct {
	ID    int64
	Chat  string
	Count int
}

func requestHistory(db *sql.DB, chat string, count int, now int64) (int64, error) {
	res, err := db.Exec(`INSERT INTO history_requests (chat, count, created) VALUES (?, ?, ?)`, chat, count, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func claimHistoryRequest(db *sql.DB) (HistoryRequest, bool, error) {
	var r HistoryRequest
	err := db.QueryRow(`UPDATE history_requests SET status = 'sending'
		WHERE id = (SELECT id FROM history_requests WHERE status = 'waiting' ORDER BY id LIMIT 1)
		RETURNING id, chat, count`).Scan(&r.ID, &r.Chat, &r.Count)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

func finishHistoryRequest(db *sql.DB, id int64, sendErr error) error {
	if sendErr != nil {
		_, err := db.Exec(`UPDATE history_requests SET status = 'failed', error = ? WHERE id = ?`, sendErr.Error(), id)
		return err
	}
	_, err := db.Exec(`UPDATE history_requests SET status = 'sent' WHERE id = ?`, id)
	return err
}

func historyRequestStatus(db *sql.DB, id int64) (status, errText string, err error) {
	err = db.QueryRow(`SELECT status, error FROM history_requests WHERE id = ?`, id).Scan(&status, &errText)
	return
}

func searchMessages(db *sql.DB, query string, limit int) ([]Message, error) {
	like := "%" + query + "%"
	return queryMessages(db, `WHERE m.text LIKE ? OR m.transcript LIKE ? OR m.file_name LIKE ? ORDER BY m.ts DESC LIMIT ?`, like, like, like, limit)
}

type Chat struct {
	JID    string `json:"jid"`
	Name   string `json:"name"`
	LastTS int64  `json:"lastTs"`
	Tone   string `json:"tone"`
}

func listChats(db *sql.DB, query string, limit int, before int64) ([]Chat, error) {
	if before <= 0 {
		before = math.MaxInt64
	}
	rows, err := db.Query(`SELECT jid, name, last_ts, tone FROM chats WHERE (name LIKE ? OR jid LIKE ?) AND last_ts < ? ORDER BY last_ts DESC LIMIT ?`,
		"%"+query+"%", "%"+query+"%", before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Chat{}
	for rows.Next() {
		var c Chat
		if err := rows.Scan(&c.JID, &c.Name, &c.LastTS, &c.Tone); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type Tone struct {
	Tone   string `json:"tone"`
	Source string `json:"source"`
}

func defaultTone(db *sql.DB) (string, error) {
	var tone string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = 'tone'`).Scan(&tone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return tone, err
}

func setDefaultTone(db *sql.DB, tone string) error {
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('tone', ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, strings.TrimSpace(tone))
	return err
}

func setChatTone(db *sql.DB, chat, tone string) error {
	_, err := db.Exec(`INSERT INTO chats (jid, tone) VALUES (?, ?) ON CONFLICT (jid) DO UPDATE SET tone = excluded.tone`, chat, strings.TrimSpace(tone))
	return err
}

func chatTone(db *sql.DB, chat string) (Tone, error) {
	var tone string
	if err := db.QueryRow(`SELECT tone FROM chats WHERE jid = ?`, chat).Scan(&tone); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Tone{}, err
	}
	if tone != "" {
		return Tone{Tone: tone, Source: "conversa"}, nil
	}
	tone, err := defaultTone(db)
	if err != nil || tone == "" {
		return Tone{Source: "nenhum"}, err
	}
	return Tone{Tone: tone, Source: "padrão"}, nil
}

type Outgoing struct {
	ID    int64
	Chat  string
	Text  string
	Audio string
	File  string
}

func enqueue(db *sql.DB, o Outgoing, now int64) (int64, error) {
	res, err := db.Exec(`INSERT INTO outbox (chat, text, audio, file, created) VALUES (?, ?, ?, ?, ?)`, o.Chat, o.Text, o.Audio, o.File, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func outboxStatus(db *sql.DB, id int64) (status, errText string, err error) {
	err = db.QueryRow(`SELECT status, error FROM outbox WHERE id = ?`, id).Scan(&status, &errText)
	return
}

func claimNextOutgoing(db *sql.DB) (Outgoing, bool, error) {
	var o Outgoing
	err := db.QueryRow(`UPDATE outbox SET status = 'sending'
		WHERE id = (SELECT id FROM outbox WHERE status = 'waiting' ORDER BY id LIMIT 1)
		RETURNING id, chat, text, audio, file`).Scan(&o.ID, &o.Chat, &o.Text, &o.Audio, &o.File)
	if errors.Is(err, sql.ErrNoRows) {
		return o, false, nil
	}
	return o, err == nil, err
}

func finishOutgoing(db *sql.DB, id int64, sendErr error) error {
	if sendErr != nil {
		_, err := db.Exec(`UPDATE outbox SET status = 'failed', error = ? WHERE id = ?`, sendErr.Error(), id)
		return err
	}
	_, err := db.Exec(`UPDATE outbox SET status = 'sent' WHERE id = ?`, id)
	return err
}

var phoneDigits = regexp.MustCompile(`\D`)

func normalizeChat(chat string) (string, error) {
	chat = strings.TrimSpace(chat)
	if strings.Contains(chat, "@") {
		if strings.HasSuffix(chat, "@s.whatsapp.net") || strings.HasSuffix(chat, "@g.us") || strings.HasSuffix(chat, "@lid") {
			return chat, nil
		}
		return "", fmt.Errorf("JID inválido: %s", chat)
	}
	digits := phoneDigits.ReplaceAllString(chat, "")
	if len(digits) < 10 || len(digits) > 15 {
		return "", fmt.Errorf("número inválido: use DDI + DDD + número, por exemplo 5562999998888")
	}
	return digits + "@s.whatsapp.net", nil
}
