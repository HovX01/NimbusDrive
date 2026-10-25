package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestBackup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/api/v1/backups/telegram" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-Nimbus-Access") != "access" {
			t.Errorf("access header = %q", r.Header.Get("X-Nimbus-Access"))
		}
		if r.Header.Get("X-Nimbus-Key") != "key" {
			t.Errorf("api key header = %q", r.Header.Get("X-Nimbus-Key"))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"status":"uploaded","name":"nimbus-db-backup-test.sqlite"}`)
	}))
	defer srv.Close()

	body, err := requestBackup(context.Background(), srv.URL, "access", "key")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"status":"uploaded","name":"nimbus-db-backup-test.sqlite"}` {
		t.Fatalf("body = %s", body)
	}
}

func TestRequestBackupSurfacesFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "telegram not authorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := requestBackup(context.Background(), srv.URL, "", ""); err == nil {
		t.Fatal("expected an error")
	}
}
