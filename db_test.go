package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestSearchFindsTranscriptAndOutboxSendsOnce(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	chat := "5562999998888@s.whatsapp.net"
	if err := saveMessage(db, Message{Chat: chat, ChatName: "Ana", ID: "a1", TS: 10, File: "/x.ogg", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE messages SET transcript = 'reunião amanhã às nove', status = 'done' WHERE id = 'a1'`); err != nil {
		t.Fatal(err)
	}
	found, err := searchMessages(db, "amanhã", 10)
	if err != nil || len(found) != 1 || found[0].ChatName != "Ana" {
		t.Fatalf("search = %+v, %v", found, err)
	}

	id, err := enqueue(db, Outgoing{Chat: chat, Text: "oi"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if o, ok, err := claimNextOutgoing(db); err != nil || !ok || o.ID != id || o.Text != "oi" {
		t.Fatalf("claim = %+v, %v, %v", o, ok, err)
	}
	if _, ok, _ := claimNextOutgoing(db); ok {
		t.Fatal("a send must be claimed only once")
	}
	if err := finishOutgoing(db, id, errors.New("offline")); err != nil {
		t.Fatal(err)
	}
	if status, msg, _ := outboxStatus(db, id); status != "failed" || msg != "offline" {
		t.Fatalf("status = %s %s", status, msg)
	}
}

func TestHistoryRequestPagesBeforeTheOldestMessage(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	chat := "5562999998888@s.whatsapp.net"
	if _, err := oldestMessage(db, chat); err == nil {
		t.Fatal("a chat without messages has no anchor")
	}
	for i, ts := range []int64{30, 10, 20} {
		if err := saveMessage(db, Message{Chat: chat, ID: string(rune('a' + i)), TS: ts, Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if oldest, err := oldestMessage(db, chat); err != nil || oldest.TS != 10 {
		t.Fatalf("oldest = %+v, %v", oldest, err)
	}
	page, err := recentMessages(db, chat, 10, 30)
	if err != nil || len(page) != 2 || page[0].TS != 20 {
		t.Fatalf("page = %+v, %v", page, err)
	}
	if n, _ := countBefore(db, chat, 20); n != 1 {
		t.Fatalf("countBefore = %d", n)
	}

	id, _ := requestHistory(db, chat, 50, 1)
	req, ok, err := claimHistoryRequest(db)
	if err != nil || !ok || req.ID != id || req.Count != 50 {
		t.Fatalf("claim = %+v, %v, %v", req, ok, err)
	}
	if _, ok, _ := claimHistoryRequest(db); ok {
		t.Fatal("a request must be claimed only once")
	}
	if err := finishHistoryRequest(db, id, nil); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := historyRequestStatus(db, id); status != "sent" {
		t.Fatalf("status = %s", status)
	}
}

func TestNormalizeChat(t *testing.T) {
	for in, want := range map[string]string{
		"+55 (62) 99999-8888":          "5562999998888@s.whatsapp.net",
		"120363000000000000@g.us":      "120363000000000000@g.us",
		"5562999998888@s.whatsapp.net": "5562999998888@s.whatsapp.net",
	} {
		if got, err := normalizeChat(in); err != nil || got != want {
			t.Errorf("normalizeChat(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"123", "ana@gmail.com", ""} {
		if _, err := normalizeChat(bad); err == nil {
			t.Errorf("normalizeChat(%q) must fail", bad)
		}
	}
}
