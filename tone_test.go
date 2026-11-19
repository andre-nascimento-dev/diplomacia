package main

import (
	"path/filepath"
	"testing"
)

func TestChatToneFallsBackToTheDefault(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	chat := "5562999998888@s.whatsapp.net"
	check := func(want Tone) {
		t.Helper()
		if got, err := chatTone(db, chat); err != nil || got != want {
			t.Fatalf("chatTone = %+v, %v; want %+v", got, err, want)
		}
	}

	check(Tone{Source: "nenhum"})
	if err := setDefaultTone(db, " Formal "); err != nil {
		t.Fatal(err)
	}
	check(Tone{Tone: "Formal", Source: "padrão"})
	if err := setChatTone(db, chat, "Informal"); err != nil {
		t.Fatal(err)
	}
	check(Tone{Tone: "Informal", Source: "conversa"})

	if err := saveMessage(db, Message{Chat: chat, ChatName: "Ana", ID: "a1", TS: 10}); err != nil {
		t.Fatal(err)
	}
	check(Tone{Tone: "Informal", Source: "conversa"})
	chats, err := listChats(db, "Ana", 10, 0)
	if err != nil || len(chats) != 1 || chats[0].Tone != "Informal" {
		t.Fatalf("listChats = %+v, %v", chats, err)
	}

	if err := setChatTone(db, chat, ""); err != nil {
		t.Fatal(err)
	}
	check(Tone{Tone: "Formal", Source: "padrão"})
}
