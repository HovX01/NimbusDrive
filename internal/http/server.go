package httpserver

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/vrc/nimbus/internal/app"
	"github.com/vrc/nimbus/internal/domain"
)

type Server struct {
	svc              *app.Services
	corsOrigins      []string
	accessSecret     string
	apiKey           string
	requireAccessKey bool
	webDir           string
	s3Port           string
}

func New(svc *app.Services, corsOrigins []string, accessSecret, apiKey string, public bool, webDir, s3Port string) *Server {
	return &Server{
		svc:              svc,
		corsOrigins:      corsOrigins,
		accessSecret:     accessSecret,
		apiKey:           apiKey,
		requireAccessKey: !public,
		webDir:           strings.TrimSpace(webDir),
		s3Port:           strings.TrimSpace(s3Port),
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(2 * time.Hour))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Range", "X-Nimbus-Access", "X-Nimbus-Key"},
		ExposedHeaders:   []string{"Accept-Ranges", "Content-Range", "Content-Length"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "nimbus"})
	})
	if s.webDir == "" {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{
				"service": "nimbus",
				"ui":      "http://localhost:5173",
				"health":  "/health",
				"api":     "/api/v1",
				"docs":    "/docs/",
			})
		})
	}

	r.Handle("/docs/*", http.StripPrefix("/docs/", http.FileServer(http.Dir("docs"))))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/share/{token}", s.shareInfo)
		r.Get("/share/{token}/download", s.shareDownload)
		r.Get("/setup/status", s.setupStatus)
		r.Get("/social/oauth/{provider}/callback", s.socialOAuthCallback)

		r.Group(func(r chi.Router) {
			r.Use(s.accessRequired)
			r.Post("/setup/telegram", s.setupTelegram)
			r.Post("/auth/send-code", s.sendCode)
			r.Post("/auth/sign-in", s.signIn)
			r.Post("/auth/resume", s.resume)

			r.Group(func(r chi.Router) {
				r.Use(s.apiKeyOrSessionRequired)
				r.Get("/contacts", s.listContacts)
				r.Get("/social/connections", s.listSocialConnections)
				r.Get("/social/oauth/config", s.listSocialOAuthConfigs)
				r.Put("/social/oauth/config/{provider}", s.configureSocialOAuthProvider)
				r.Get("/social/oauth/{provider}/start", s.startSocialOAuth)
				r.Post("/social/connections/{provider}/cookies", s.addSocialCookieConnection)
				r.Patch("/social/connections/{provider}", s.patchSocialConnection)
				r.Post("/social/connections/{provider}/{id}/active", s.setActiveSocialConnection)
				r.Delete("/social/connections/{provider}/{id}", s.deleteSocialConnection)
				r.Get("/contacts/{id}/avatar", s.contactAvatar)
				r.Get("/bots", s.listBots)
				r.Patch("/bots/{id}", s.patchBot)
				r.Get("/files/media/{jobID}", s.mediaJobStatus)
				r.Get("/edit/projects", s.listEditProjects)
				r.Get("/edit/projects/{projectID}", s.getEditProject)
				r.Patch("/edit/projects/{projectID}", s.saveEditTimeline)
				r.Delete("/edit/projects/{projectID}", s.deleteEditProject)
				r.Post("/edit/projects/{projectID}/export", s.startEditExport)
				r.Post("/edit/projects/{projectID}/captions/import", s.importCaptions)
				r.Get("/edit/projects/{projectID}/captions/export", s.exportCaptions)
				r.Get("/edit/jobs/{jobID}", s.editExportStatus)
				r.Get("/files", s.listFiles)
				r.Get("/data/collections", s.listDataCollections)
				r.Get("/data/{collection}", s.queryData)
				r.Post("/data/{collection}", s.insertData)
				r.Get("/data/{collection}/{id}", s.getDataRow)
				r.Patch("/data/{collection}/{id}", s.updateData)
				r.Delete("/data/{collection}/{id}", s.deleteData)
				r.Get("/search", s.search)
				r.Post("/folders", s.mkdir)
				r.Post("/files/upload", s.upload)
				r.Post("/files/import", s.importURL)
				r.Post("/files/fetch", s.fetchURL)
				r.Get("/files/fetch/{jobID}", s.fetchJobStatus)
				r.Get("/files/{id}/download", s.download)
				r.Get("/files/{id}/probe", s.probeFile)
				r.Get("/files/{id}/keyframes", s.getKeyframes)
				r.Get("/files/{id}/proxy", s.serveEditorProxy)
				r.Get("/files/{id}/proxy/status", s.proxyStatus)
				r.Post("/files/{id}/send-telegram", s.sendTelegram)
				r.Post("/files/{id}/media", s.startMedia)
				r.Post("/files/{id}/edit/projects", s.createEditProject)
				r.Get("/files/{id}/thumb", s.thumb)
				r.Patch("/files/{id}", s.rename)
				r.Post("/files/{id}/move", s.moveFile)
				r.Post("/files/move", s.moveFiles)
				r.Delete("/files/{id}", s.delete)
				r.Get("/trash", s.listTrash)
				r.Delete("/trash", s.emptyTrash)
				r.Delete("/trash/{id}", s.purgeTrash)
				r.Post("/files/{id}/share", s.createShare)
				r.Get("/files/{id}/shares", s.listShares)
				r.Delete("/shares/{shareID}", s.revokeShare)
			})

r.Group(func(r chi.Router) {
				r.Use(s.sessionRequired)
			r.Get("/settings/backup", s.getBackupSettings)
			r.Put("/settings/backup", s.saveBackupSettings)
			r.Post("/settings/backup/test", s.testBackupSettings)
			r.Get("/settings/s3", s.getS3Settings)
			r.Put("/settings/s3", s.saveS3Settings)
			r.Get("/s3/buckets", s.listS3Buckets)
			r.Post("/s3/buckets", s.createS3Bucket)
			r.Get("/s3/buckets/{bucket}", s.listS3BucketObjects)
				r.Post("/backups", s.startBackup)
				r.Get("/backups/{jobID}", s.backupStatus)
				r.Get("/auth/me", s.me)
				r.Get("/auth/avatar", s.avatar)
				r.Post("/auth/logout", s.logout)
				r.Get("/settings/storage", s.storageSettings)
			})
		})
	})

	if s.webDir != "" {
		r.NotFound(spaHandler(s.webDir))
	}

	return r
}

func spaHandler(webDir string) http.HandlerFunc {
	root := http.Dir(webDir)
	fileServer := http.FileServer(root)
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/docs/") {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
		if path == "." || path == "" {
			http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
			return
		}
		f, err := root.Open(path)
		if err != nil {
			if os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist) {
				http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		_ = f.Close()
		fileServer.ServeHTTP(w, r)
	}
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	configured, authorized, err := s.svc.SetupStatus(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":          configured,
		"authorized":          authorized,
		"access_key_required": s.requireAccessKey,
		"api_key_configured":  s.apiKey != "",
	})
}

func (s *Server) setupTelegram(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIID   int    `json:"api_id"`
		APIHash string `json:"api_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	if err := s.svc.ConfigureTelegram(r.Context(), body.APIID, body.APIHash); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) sendCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	hash, err := s.svc.SendCode(r.Context(), body.Phone)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"phone_code_hash": hash})
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone         string `json:"phone"`
		Code          string `json:"code"`
		PhoneCodeHash string `json:"phone_code_hash"`
		Password      string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	profile, token, err := s.svc.SignIn(r.Context(), body.Phone, body.Code, body.PhoneCodeHash, body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": profile})
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	profile, token, err := s.svc.ResumeSession(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": profile})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	profile, err := s.svc.Me(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) avatar(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := s.svc.Avatar(r.Context(), &buf); err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Logout(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) storageSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"api_key":  s.apiKey,
		"api_base": publicBase(r),
	})
}

func intQuery(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	parent := r.URL.Query().Get("parent_id")
	var (
		nodes []domain.Node
		err   error
	)
	if r.URL.Query().Get("recursive") == "true" {
		nodes, err = s.svc.ListAll(r.Context(), intQuery(r, "limit", 1000))
	} else {
		nodes, err = s.svc.List(r.Context(), parent)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if nodes == nil {
		nodes = []domain.Node{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nodes})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	hits, err := s.svc.Search(r.Context(), q, 50)
	if err != nil {
		writeErr(w, err)
		return
	}
	if hits == nil {
		hits = []domain.SearchHit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": hits})
}

func (s *Server) mkdir(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	node, err := s.svc.Mkdir(r.Context(), body.ParentID, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, node)
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	// Stream multipart; spill to disk above 32MiB so huge files are not held in RAM.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, fmt.Errorf("%w: invalid multipart upload: %v", domain.ErrValidation, err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, fmt.Errorf("%w: file field required", domain.ErrValidation))
		return
	}
	defer file.Close()
	parent := r.FormValue("parent_id")
	contentType := header.Header.Get("Content-Type")
	node, err := s.svc.Upload(r.Context(), parent, header.Filename, contentType, file, header.Size)
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusCreated
	if node.Duplicate {
		status = http.StatusOK
	}
	s.writeFileResponse(w, r, status, node, wantsPublicURL(r))
}

func (s *Server) importURL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL      string `json:"url"`
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
		Public   bool   `json:"public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		writeErr(w, fmt.Errorf("%w: url required", domain.ErrValidation))
		return
	}
	node, err := s.svc.ImportURL(r.Context(), body.ParentID, body.URL, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	withURL := body.Public || wantsPublicURL(r)
	s.writeFileResponse(w, r, http.StatusCreated, node, withURL)
}

func (s *Server) fetchURL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL       string `json:"url"`
		ParentID  string `json:"parent_id"`
		Mode      string `json:"mode"`
		MaxHeight int    `json:"max_height"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	jobID, err := s.svc.StartFetchURL(body.ParentID, body.URL, body.Mode, body.MaxHeight)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (s *Server) fetchJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	st, err := s.svc.GetFetchJob(jobID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) rename(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	node, err := s.svc.Rename(r.Context(), id, body.Name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) moveFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	node, err := s.svc.Move(r.Context(), id, body.ParentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) moveFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs      []string `json:"ids"`
		ParentID string   `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrValidation)
		return
	}
	if len(body.IDs) == 0 {
		writeErr(w, domain.ErrValidation)
		return
	}
	if err := s.svc.MoveMany(r.Context(), body.IDs, body.ParentID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	meta, err := s.svc.GetNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if meta.Type != domain.NodeFile || meta.Status != domain.StatusReady {
		writeErr(w, domain.ErrNotFound)
		return
	}

	ctype := downloadContentType(meta.MimeType, meta.Name)
	name := domain.SanitizeDownloadName(meta.Name)
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")

	start, end, ok, err := parseByteRange(r.Header.Get("Range"), meta.Size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", meta.Size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if !ok {
		w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
		if _, err := s.svc.Download(r.Context(), id, w); err != nil {
			return
		}
		return
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, meta.Size))
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.WriteHeader(http.StatusPartialContent)
	if _, err := s.svc.DownloadRange(r.Context(), id, w, start, end); err != nil {
		return
	}
}

func (s *Server) thumb(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b, err := s.svc.ThumbBytes(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	_, _ = w.Write(b)
}

// parseByteRange handles a single "bytes=start-end" request.
// ok=false means no Range header (send full body).
func parseByteRange(header string, size int64) (start, end int64, ok bool, err error) {
	if header == "" || size <= 0 {
		return 0, 0, false, nil
	}
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false, domain.ErrValidation
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(spec, ",") {
		// Multi-range not supported.
		return 0, 0, false, domain.ErrValidation
	}
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false, domain.ErrValidation
	}

	if parts[0] == "" {
		// bytes=-suffix
		suffix, convErr := strconv.ParseInt(parts[1], 10, 64)
		if convErr != nil || suffix <= 0 {
			return 0, 0, false, domain.ErrValidation
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true, nil
	}

	start, convErr := strconv.ParseInt(parts[0], 10, 64)
	if convErr != nil || start < 0 {
		return 0, 0, false, domain.ErrValidation
	}
	if parts[1] == "" {
		end = size - 1
	} else {
		end, convErr = strconv.ParseInt(parts[1], 10, 64)
		if convErr != nil {
			return 0, 0, false, domain.ErrValidation
		}
	}
	if end >= size {
		end = size - 1
	}
	if start > end {
		return 0, 0, false, domain.ErrValidation
	}
	return start, end, true, nil
}

func downloadContentType(mimeType, name string) string {
	if mimeType != "" && mimeType != "application/octet-stream" {
		return mimeType
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	default:
		if mimeType != "" {
			return mimeType
		}
		return "application/octet-stream"
	}
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "trashed"})
}

func (s *Server) listContacts(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.ListContacts(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listBots(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.ListBots(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if items == nil {
		items = []domain.TelegramBot{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) patchBot(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "invalid bot id"}})
		return
	}
	var body struct {
		Allowed *bool `json:"allowed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Allowed == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "expected {\"allowed\": true|false}"}})
		return
	}
	bot, err := s.svc.SetBotAllowed(r.Context(), id, *body.Allowed)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bot)
}

func (s *Server) contactAvatar(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "invalid contact id"}})
		return
	}
	var buf bytes.Buffer
	if err := s.svc.ContactAvatar(r.Context(), id, &buf); err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) sendTelegram(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID int64 `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "invalid json"}})
		return
	}
	if err := s.svc.SendToTelegram(r.Context(), chi.URLParam(r, "id"), body.UserID); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) startMedia(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "invalid json"}})
		return
	}
	jobID, err := s.svc.StartMediaJob(r.Context(), chi.URLParam(r, "id"), body.Action)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (s *Server) mediaJobStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.svc.MediaJobStatus(chi.URLParam(r, "jobID"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) listTrash(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.ListTrash(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) purgeTrash(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Purge(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "purged"})
}

func (s *Server) emptyTrash(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.EmptyTrash(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) createShare(w http.ResponseWriter, r *http.Request) {
	info, err := s.svc.CreateShare(r.Context(), chi.URLParam(r, "id"), publicBase(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (s *Server) listShares(w http.ResponseWriter, r *http.Request) {
	items, err := s.svc.ListShares(r.Context(), chi.URLParam(r, "id"), publicBase(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) revokeShare(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.RevokeShare(r.Context(), chi.URLParam(r, "shareID")); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) shareInfo(w http.ResponseWriter, r *http.Request) {
	node, link, err := s.svc.GetSharedFile(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":      node.Name,
		"mime_type": node.MimeType,
		"size":      node.Size,
		"shared_at": link.CreatedAt,
	})
}

func (s *Server) shareDownload(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	node, link, err := s.svc.GetSharedFile(r.Context(), token)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctype := downloadContentType(node.MimeType, node.Name)
	name := domain.SanitizeDownloadName(node.Name)
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(node.Size, 10))
	if _, err := s.svc.Download(r.Context(), node.ID, w); err != nil {
		return
	}
	_ = s.svc.IncrementShareDownload(r.Context(), link.ID)
}

func publicBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:9090"
	}
	return scheme + "://" + host
}

func (s *Server) accessRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireAccessKey {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(r.Header.Get("X-Nimbus-Access"))
		if got == "" {
			got = strings.TrimSpace(r.URL.Query().Get("access"))
		}
		if !secureEqual(s.accessSecret, got) {
			writeErr(w, fmt.Errorf("%w: missing or invalid access key (X-Nimbus-Access)", domain.ErrUnauthorized))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeFileResponse(w http.ResponseWriter, r *http.Request, status int, node domain.Node, withURL bool) {
	if !withURL {
		writeJSON(w, status, node)
		return
	}
	info, err := s.svc.CreateShare(r.Context(), node.ID, publicBase(r))
	if err != nil {
		writeJSON(w, status, node)
		return
	}
	writeJSON(w, status, app.StoredFile{Node: node, URL: info.URL})
}

func secureEqual(want, got string) bool {
	if want == "" || got == "" {
		return false
	}
	a := []byte(want)
	b := []byte(got)
	if len(a) != len(b) {
		// Compare against itself so timing still depends on length of want.
		_ = subtle.ConstantTimeCompare(a, a)
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	switch {
	case errors.Is(err, domain.ErrValidation):
		status, code = http.StatusBadRequest, "validation_error"
	case errors.Is(err, domain.ErrNotConfigured):
		status, code = http.StatusPreconditionRequired, "not_configured"
	case errors.Is(err, domain.ErrProviderConfig):
		status, code = http.StatusPreconditionRequired, "provider_not_configured"
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrNotAuthenticated):
		status, code = http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrConflict):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrTwoFARequired):
		status, code = http.StatusUnauthorized, "two_fa_required"
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": err.Error()},
	})
}
