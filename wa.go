package main

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type WA struct {
	db       *sql.DB
	mediaDir string
	cli      *whatsmeow.Client

	historyMu sync.Mutex
	mu        sync.Mutex
	status    string
	qr        string
}

func openDevice(ctx context.Context, storePath string) (*store.Device, error) {
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+storePath+"?_foreign_keys=on&_busy_timeout=5000", waLog.Noop)
	if err != nil {
		return nil, err
	}
	return container.GetFirstDevice(ctx)
}

func logout(ctx context.Context, storePath string) error {
	device, err := openDevice(ctx, storePath)
	if err != nil {
		return err
	}
	if device.ID == nil {
		return nil
	}
	cli := whatsmeow.NewClient(device, waLog.Noop)
	if err := cli.Connect(); err != nil {
		return err
	}
	defer cli.Disconnect()
	for i := 0; i < 30 && !cli.IsLoggedIn(); i++ {
		time.Sleep(500 * time.Millisecond)
	}
	return cli.Logout(ctx)
}

func startWA(ctx context.Context, db *sql.DB, storePath, mediaDir string) (*WA, error) {
	store.DeviceProps.RequireFullSync = proto.Bool(true)
	device, err := openDevice(ctx, storePath)
	if err != nil {
		return nil, err
	}
	w := &WA{db: db, mediaDir: mediaDir, cli: whatsmeow.NewClient(device, waLog.Noop), status: "Conectando…"}
	w.cli.AddEventHandler(w.handle)

	if w.cli.Store.ID == nil {
		qrs, err := w.cli.GetQRChannel(ctx)
		if err != nil {
			return nil, err
		}
		go w.watchQR(qrs)
	}
	if err := w.cli.Connect(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *WA) watchQR(qrs <-chan whatsmeow.QRChannelItem) {
	for item := range qrs {
		switch item.Event {
		case "code":
			png, err := qrcode.Encode(item.Code, qrcode.Medium, 320)
			if err != nil {
				w.setState("Falha ao gerar o QR code: "+err.Error(), "")
				continue
			}
			w.setState("Escaneie o QR code no WhatsApp do celular: Aparelhos conectados > Conectar aparelho.",
				"data:image/png;base64,"+base64.StdEncoding.EncodeToString(png))
		case "success":
			w.setState("Conectado.", "")
		case "timeout":
			w.setState("O QR code expirou. Feche e abra o DiplomacIA de novo.", "")
		default:
			if item.Error != nil {
				w.setState("Falha no vínculo: "+item.Error.Error(), "")
			}
		}
	}
}

func (w *WA) setState(status, qr string) {
	w.mu.Lock()
	w.status, w.qr = status, qr
	w.mu.Unlock()
}

func (w *WA) State() (string, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status, w.qr
}

func (w *WA) handle(evt any) {
	switch e := evt.(type) {
	case *events.Connected:
		w.setState("Conectado.", "")
	case *events.Disconnected:
		w.setState("Sem conexão. O DiplomacIA tenta conectar de novo.", "")
	case *events.LoggedOut:
		w.setState("O celular desconectou o DiplomacIA. Feche e abra o app para ver um QR code novo.", "")
		_ = w.cli.Store.Delete(context.Background())
	case *events.Message:
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			w.store(ctx, e)
		}()
	case *events.HistorySync:
		go w.storeHistory(e)
	}
}

func (w *WA) storeHistory(e *events.HistorySync) {
	w.historyMu.Lock()
	defer w.historyMu.Unlock()
	for _, conv := range e.Data.GetConversations() {
		chat, err := types.ParseJID(conv.GetID())
		if err != nil {
			continue
		}
		for _, hm := range conv.GetMessages() {
			msg, err := w.cli.ParseWebMessage(chat, hm.GetMessage())
			if err != nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			w.store(ctx, msg)
			cancel()
		}
	}
}

func (w *WA) store(ctx context.Context, e *events.Message) {

	m := Message{
		Chat:       w.phoneJID(ctx, e.Info.Chat).String(),
		ID:         e.Info.ID,
		SenderName: e.Info.PushName,
		FromMe:     e.Info.IsFromMe,
		TS:         e.Info.Timestamp.Unix(),
	}
	if m.FromMe {
		m.SenderName = "Você"
	}
	msg := e.Message
	switch {
	case msg.GetConversation() != "":
		m.Text = msg.GetConversation()
	case msg.GetExtendedTextMessage().GetText() != "":
		m.Text = msg.GetExtendedTextMessage().GetText()
	default:
		media, ok, err := describeMedia(msg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "describe media:", err)
		}
		if !ok {
			return
		}
		m.Text, m.Kind, m.Mime, m.FileName, m.Size, m.Seconds, m.Media =
			media.Text, media.Kind, media.Mime, media.FileName, media.Size, media.Seconds, media.Media
		if m.Kind == "audio" {
			file, err := w.download(ctx, m)
			m.File = file
			m.Status, m.Transcript = afterDownload(m.Kind, err)
		}
	}
	m.ChatName = w.chatName(ctx, e.Info.Chat, m)
	if err := saveMessage(w.db, m); err != nil {
		fmt.Fprintln(os.Stderr, "save message:", err)
	}
}

type mediaPart interface {
	GetMimetype() string
	GetFileLength() uint64
}

func describeMedia(msg *waE2E.Message) (Message, bool, error) {
	var m Message
	var part mediaPart
	wrap := &waE2E.Message{}
	switch {
	case msg.GetImageMessage() != nil:
		wrap.ImageMessage = msg.GetImageMessage()
		part, m.Kind, m.Text = wrap.ImageMessage, "image", wrap.ImageMessage.GetCaption()
	case msg.GetVideoMessage() != nil || msg.GetPtvMessage() != nil:
		wrap.VideoMessage = cmp.Or(msg.GetVideoMessage(), msg.GetPtvMessage())
		part, m.Kind, m.Text, m.Seconds = wrap.VideoMessage, "video", wrap.VideoMessage.GetCaption(), int(wrap.VideoMessage.GetSeconds())
	case msg.GetAudioMessage() != nil:
		wrap.AudioMessage = msg.GetAudioMessage()
		part, m.Kind, m.Seconds = wrap.AudioMessage, "audio", int(wrap.AudioMessage.GetSeconds())
	case msg.GetDocumentMessage() != nil:
		wrap.DocumentMessage = msg.GetDocumentMessage()
		part, m.Kind, m.Text, m.FileName = wrap.DocumentMessage, "document", wrap.DocumentMessage.GetCaption(), wrap.DocumentMessage.GetFileName()
	case msg.GetStickerMessage() != nil:
		wrap.StickerMessage = msg.GetStickerMessage()
		part, m.Kind = wrap.StickerMessage, "sticker"
	default:
		return m, false, nil
	}
	m.Mime, m.Size = part.GetMimetype(), int64(part.GetFileLength())
	media, err := proto.Marshal(wrap)
	m.Media = media
	return m, true, err
}

func (w *WA) download(ctx context.Context, m Message) (string, error) {
	var msg waE2E.Message
	if err := proto.Unmarshal(m.Media, &msg); err != nil {
		return "", err
	}
	data, err := w.cli.DownloadAny(ctx, &msg)
	if err != nil {
		return "", err
	}
	path := filepath.Join(w.mediaDir, safeName(m.ID)+fileExt(m.Mime, m.FileName))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

var commonExt = map[string]string{"image/jpeg": ".jpg", "image/webp": ".webp", "audio/ogg": ".ogg", "audio/mpeg": ".mp3",
	"audio/mp4": ".m4a", "video/mp4": ".mp4", "application/pdf": ".pdf"}

func fileExt(mimeType, fileName string) string {
	if ext := filepath.Ext(fileName); ext != "" {
		return ext
	}
	base, _, _ := mime.ParseMediaType(mimeType)
	if ext, ok := commonExt[base]; ok {
		return ext
	}
	if exts, _ := mime.ExtensionsByType(base); len(exts) > 0 {
		return exts[0]
	}
	return ".bin"
}

func (w *WA) queueLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
		if !w.cli.IsLoggedIn() {
			continue
		}
		if o, ok, err := claimNextOutgoing(w.db); err == nil && ok {
			sendCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			if err := finishOutgoing(w.db, o.ID, w.Send(sendCtx, o)); err != nil {
				fmt.Fprintln(os.Stderr, "outbox:", err)
			}
			cancel()
		}
		if m, ok, err := nextDownload(w.db); err == nil && ok {
			dlCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			file, dlErr := w.download(dlCtx, m)
			cancel()
			if err := finishDownload(w.db, m, file, dlErr); err != nil {
				fmt.Fprintln(os.Stderr, "download:", err)
			}
		}
		if req, ok, err := claimHistoryRequest(w.db); err == nil && ok {
			if err := finishHistoryRequest(w.db, req.ID, w.askHistory(ctx, req)); err != nil {
				fmt.Fprintln(os.Stderr, "history request:", err)
			}
		}
	}
}

func (w *WA) askHistory(ctx context.Context, req HistoryRequest) error {
	oldest, err := oldestMessage(w.db, req.Chat)
	if err != nil {
		return err
	}
	chat, err := types.ParseJID(req.Chat)
	if err != nil {
		return err
	}
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, IsFromMe: oldest.FromMe},
		ID:            oldest.ID,
		Timestamp:     time.Unix(oldest.TS, 0),
	}
	_, err = w.cli.SendPeerMessage(ctx, w.cli.BuildHistorySyncRequest(info, req.Count))
	return err
}

func (w *WA) phoneJID(ctx context.Context, jid types.JID) types.JID {
	if jid.Server != types.HiddenUserServer {
		return jid.ToNonAD()
	}
	if pn, err := w.cli.Store.LIDs.GetPNForLID(ctx, jid); err == nil && !pn.IsEmpty() {
		return pn.ToNonAD()
	}
	return jid.ToNonAD()
}

func (w *WA) chatName(ctx context.Context, chat types.JID, m Message) string {
	var known string
	if w.db.QueryRow(`SELECT name FROM chats WHERE jid = ?`, m.Chat).Scan(&known); known != "" {
		return known
	}
	if chat.Server == types.GroupServer {
		if info, err := w.cli.GetGroupInfo(ctx, chat); err == nil {
			return info.Name
		}
		return ""
	}
	if c, err := w.cli.Store.Contacts.GetContact(ctx, w.phoneJID(ctx, chat)); err == nil {
		for _, name := range []string{c.FullName, c.FirstName, c.PushName, c.BusinessName} {
			if name != "" {
				return name
			}
		}
	}
	if !m.FromMe {
		return m.SenderName
	}
	return ""
}

func (w *WA) Send(ctx context.Context, o Outgoing) error {
	if !w.cli.IsLoggedIn() {
		return fmt.Errorf("o DiplomacIA não está conectado ao WhatsApp")
	}
	to, err := types.ParseJID(o.Chat)
	if err != nil {
		return err
	}
	msg := &waE2E.Message{Conversation: proto.String(o.Text)}
	sent := Message{Text: o.Text}
	switch {
	case o.Audio != "":
		sent = Message{Kind: "audio", Mime: "audio/ogg", File: o.Audio}
		if msg, sent.Seconds, err = w.voiceNote(ctx, o.Audio); err != nil {
			return err
		}
	case o.File != "":
		if msg, err = w.fileMessage(ctx, o.File, o.Text); err != nil {
			return err
		}
		if sent, _, err = describeMedia(msg); err != nil {
			return err
		}
		sent.File = o.File
	}
	resp, err := w.cli.SendMessage(ctx, to, msg)
	if err != nil {
		return err
	}
	sent.Chat, sent.ID, sent.SenderName, sent.FromMe, sent.TS = o.Chat, resp.ID, "Você", true, resp.Timestamp.Unix()
	return saveMessage(w.db, sent)
}

func (w *WA) fileMessage(ctx context.Context, path, caption string) (*waE2E.Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mimeType := cmp.Or(mime.TypeByExtension(filepath.Ext(path)), http.DetectContentType(data))
	mediaType := whatsmeow.MediaDocument
	switch base, _, _ := mime.ParseMediaType(mimeType); base {
	case "image/jpeg", "image/png":
		mediaType = whatsmeow.MediaImage
	case "video/mp4":
		mediaType = whatsmeow.MediaVideo
	}
	up, err := w.cli.Upload(ctx, data, mediaType)
	if err != nil {
		return nil, err
	}
	switch mediaType {
	case whatsmeow.MediaImage:
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath),
			MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
			Mimetype: proto.String(mimeType), Caption: proto.String(caption)}}, nil
	case whatsmeow.MediaVideo:
		return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath),
			MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
			Mimetype: proto.String(mimeType), Caption: proto.String(caption)}}, nil
	}
	return &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath),
		MediaKey: up.MediaKey, FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		Mimetype: proto.String(mimeType), Caption: proto.String(caption), FileName: proto.String(filepath.Base(path)),
		Title: proto.String(filepath.Base(path))}}, nil
}

func (w *WA) voiceNote(ctx context.Context, path string) (*waE2E.Message, int, error) {
	ogg, err := os.CreateTemp("", "diplomacia-*.ogg")
	if err != nil {
		return nil, 0, err
	}
	ogg.Close()
	defer os.Remove(ogg.Name())
	if out, err := exec.CommandContext(ctx, tool("ffmpeg"), "-y", "-i", path, "-vn", "-c:a", "libopus", "-b:a", "32k",
		"-ac", "1", "-ar", "48000", "-f", "ogg", ogg.Name()).CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg: %v: %s", err, lastLine(out))
	}
	data, err := os.ReadFile(ogg.Name())
	if err != nil {
		return nil, 0, err
	}
	up, err := w.cli.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return nil, 0, err
	}
	seconds := audioSeconds(ctx, ogg.Name())
	return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
		URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
		FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		Mimetype: proto.String("audio/ogg; codecs=opus"), PTT: proto.Bool(true), Seconds: proto.Uint32(uint32(seconds)),
	}}, seconds, nil
}

func audioSeconds(ctx context.Context, path string) int {
	out, err := exec.CommandContext(ctx, tool("ffprobe"), "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0
	}
	f, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return int(f + 0.5)
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == '.' {
			return '_'
		}
		return r
	}, s)
}

func tool(name string) string {
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if p := filepath.Join(dir, name); fileExists(p) {
			return p
		}
	}
	return name
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}
