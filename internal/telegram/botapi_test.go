package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBotAPIUploadDocumentKeepsBackupNameAndCaption(t *testing.T) {
	var filename, caption, content string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/sendDocument" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			return
		}
		files := r.MultipartForm.File["document"]
		if len(files) != 1 {
			t.Errorf("expected one document, got %d", len(files))
			return
		}
		filename = files[0].Filename
		caption = r.FormValue("caption")
		f, err := files[0].Open()
		if err != nil {
			t.Errorf("open document: %v", err)
			return
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			t.Errorf("read document: %v", err)
			return
		}
		content = string(b)
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{"message_id":7}}`)
	}))
	defer srv.Close()

	client := NewBotAPI(srv.URL, "token", "123")
	msgID, err := client.UploadDocument(context.Background(), 0,
		"nimbus-db-backup-20261025T143000Z-a1b2c3d4.sqlite",
		"Nimbus database backup | sha256:abc", strings.NewReader("sqlite"), 6)
	if err != nil {
		t.Fatal(err)
	}
	if msgID != 7 {
		t.Fatalf("message id = %d", msgID)
	}
	if filename != "nimbus-db-backup-20261025T143000Z-a1b2c3d4.sqlite" {
		t.Fatalf("filename = %q", filename)
	}
	if !strings.Contains(caption, "Nimbus database backup") {
		t.Fatalf("caption = %q", caption)
	}
	if content != "sqlite" {
		t.Fatalf("content = %q", content)
	}
}

func TestBotAPIUploadPartUsesLegacyNameAndCaption(t *testing.T) {
	var filename, caption string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			return
		}
		files := r.MultipartForm.File["document"]
		if len(files) != 1 {
			t.Errorf("expected one document, got %d", len(files))
			return
		}
		filename = files[0].Filename
		caption = r.FormValue("caption")
		_, _ = fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer srv.Close()

	client := NewBotAPI(srv.URL, "token", "123")
	if _, err := client.UploadPart(context.Background(), 0, 2, "node-id", strings.NewReader("chunk"), 5); err != nil {
		t.Fatal(err)
	}
	if filename != "node-id.part0002" {
		t.Fatalf("filename = %q", filename)
	}
	if caption != "nimbus-part:2" {
		t.Fatalf("caption = %q", caption)
	}
}
