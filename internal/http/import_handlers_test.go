package httpserver

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/vrc/nimbus/internal/app"
	"github.com/vrc/nimbus/internal/domain"
)

type fakeDatabaseImporter struct {
	got   []byte
	count domain.ImportSummary
	err   error
}

func (f *fakeDatabaseImporter) MergeDatabase(_ context.Context, path string) (domain.ImportSummary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return domain.ImportSummary{}, err
	}
	f.got = b
	return f.count, f.err
}

func TestImportDatabaseUpload(t *testing.T) {
	fake := &fakeDatabaseImporter{count: domain.ImportSummary{FilesImported: 2}}
	s := &Server{svc: &app.Services{DataDir: t.TempDir(), DBImport: fake}}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "old.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, "sqlite bytes")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	s.importDatabase(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if string(fake.got) != "sqlite bytes" {
		t.Fatalf("import bytes = %q", fake.got)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"files_imported":2`)) {
		t.Fatalf("response = %s", rec.Body.String())
	}
}
