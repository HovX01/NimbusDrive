package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestFindChannelByID(t *testing.T) {
	dialogs := &tg.MessagesDialogs{
		Chats: []tg.ChatClass{
			&tg.Channel{ID: 10, Title: "Nimbus Storage"},
			&tg.Channel{ID: 42, Title: "Old storage"},
		},
	}
	if ch := findChannelByID(dialogs, 42); ch == nil || ch.ID != 42 {
		t.Fatalf("channel 42 = %+v", ch)
	}
	if ch := findChannelByID(dialogs, 99); ch != nil {
		t.Fatalf("unexpected channel = %+v", ch)
	}
}
