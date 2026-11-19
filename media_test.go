package main

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestOpenDBMigratesAnOldDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	old, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(schema + `INSERT INTO chats (jid) VALUES ('c'); INSERT INTO messages (chat, id, sender, sender_name, from_me, ts, audio, status)
		VALUES ('c', 'a1', '', 'Ana', 0, 1, '/m/a1.ogg', 'done')`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	for range 2 {
		db, err := openDB(path)
		if err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
	db, _ := openDB(path)
	defer db.Close()
	m, err := findMessage(db, "c", "a1")
	if err != nil || m.Kind != "audio" || m.File != "/m/a1.ogg" {
		t.Fatalf("migrated audio = %+v, %v", m, err)
	}
	if _, err := enqueue(db, Outgoing{Chat: "c", File: "/x.pdf"}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestDescribeMediaKeepsWhatDownloadNeeds(t *testing.T) {
	doc := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{DirectPath: proto.String("/v/doc"), FileName: proto.String("pauta.md"),
		Caption: proto.String("segue"), Mimetype: proto.String("text/markdown"), FileLength: proto.Uint64(42)}}
	m, ok, err := describeMedia(doc)
	if err != nil || !ok || m.Kind != "document" || m.FileName != "pauta.md" || m.Text != "segue" || m.Size != 42 {
		t.Fatalf("document = %+v, %v, %v", m, ok, err)
	}
	var back waE2E.Message
	if err := proto.Unmarshal(m.Media, &back); err != nil || back.GetDocumentMessage().GetDirectPath() != "/v/doc" {
		t.Fatalf("media = %v, %v", &back, err)
	}

	ptv := &waE2E.Message{PtvMessage: &waE2E.VideoMessage{DirectPath: proto.String("/v/ptv"), Seconds: proto.Uint32(9)}}
	if m, ok, _ := describeMedia(ptv); !ok || m.Kind != "video" || m.Seconds != 9 {
		t.Fatalf("video note = %+v", m)
	}
	if _, ok, _ := describeMedia(&waE2E.Message{LocationMessage: &waE2E.LocationMessage{}}); ok {
		t.Fatal("a location has no file")
	}
	if ext := fileExt("audio/ogg; codecs=opus", ""); ext != ".ogg" {
		t.Fatalf("ext = %s", ext)
	}
	if ext := fileExt("application/octet-stream", "pauta.md"); ext != ".md" {
		t.Fatalf("ext = %s", ext)
	}
}

func TestDownloadQueueFinishesEachKind(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cases := []struct {
		id, kind, status string
		err              error
	}{{"i1", "image", "done", nil}, {"v1", "video", "pending", nil}, {"d1", "document", "failed", errors.New("410")}}
	for i, c := range cases {
		if err := saveMessage(db, Message{Chat: "c", ID: c.id, TS: int64(i), Kind: c.kind, Media: []byte{1}}); err != nil {
			t.Fatal(err)
		}
		if err := requestDownload(db, "c", c.id); err != nil {
			t.Fatal(err)
		}
		m, ok, err := nextDownload(db)
		if err != nil || !ok || m.ID != c.id {
			t.Fatalf("next = %+v, %v, %v", m, ok, err)
		}
		file := "/m/" + c.id
		if c.err != nil {
			file = ""
		}
		if err := finishDownload(db, m, file, c.err); err != nil {
			t.Fatal(err)
		}
		if m, _ := findMessage(db, "c", c.id); m.Status != c.status || m.File != file {
			t.Fatalf("%s = %+v", c.id, m)
		}
	}
	if _, ok, _ := nextDownload(db); ok {
		t.Fatal("the queue must be empty")
	}
}
