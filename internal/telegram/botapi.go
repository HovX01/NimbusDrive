package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/vrc/nimbus/internal/domain"
)

type BotAPIClient struct {
	apiURL   string
	token    string
	chatID   string
	http     *http.Client
}

func NewBotAPI(apiURL, token, chatID string) *BotAPIClient {
	return &BotAPIClient{
		apiURL: strings.TrimRight(apiURL, "/"),
		token:  token,
		chatID: chatID,
		http:   &http.Client{},
	}
}

func (b *BotAPIClient) botURL(method string) string {
	return fmt.Sprintf("%s/bot%s/%s", b.apiURL, b.token, method)
}

func (b *BotAPIClient) fileURL(filePath string) string {
	return fmt.Sprintf("%s/file/bot%s/%s", b.apiURL, b.token, filePath)
}

type botResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
}

type botChat struct {
	ID int64 `json:"id"`
}

type botMessage struct {
	MessageID int `json:"message_id"`
}

type botFile struct {
	FilePath string `json:"file_path"`
}

func (b *BotAPIClient) EnsureStorageChannel(ctx context.Context) (int64, error) {
	body := fmt.Sprintf(`{"chat_id":%s}`, b.chatID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.botURL("getChat"), strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("bot api getChat: %w", err)
	}
	defer resp.Body.Close()

	var result botResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("bot api decode: %w", err)
	}
	if !result.OK {
		return 0, fmt.Errorf("bot api getChat failed for chat_id %s", b.chatID)
	}

	var chat botChat
	if err := json.Unmarshal(result.Result, &chat); err != nil {
		return 0, fmt.Errorf("bot api parse chat: %w", err)
	}
	return chat.ID, nil
}

func (b *BotAPIClient) UploadPart(ctx context.Context, channelID int64, partNo int, name string, r io.Reader, size int64) (int, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("chat_id", b.chatID); err != nil {
		return 0, err
	}
	if err := w.WriteField("caption", fmt.Sprintf("nimbus-part:%d", partNo)); err != nil {
		return 0, err
	}

	filename := fmt.Sprintf("%s.part%04d", name, partNo)
	part, err := w.CreateFormFile("document", filename)
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.botURL("sendDocument"), &buf)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := b.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("bot api sendDocument: %w", err)
	}
	defer resp.Body.Close()

	var result botResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("bot api decode: %w", err)
	}
	if !result.OK {
		return 0, fmt.Errorf("bot api sendDocument failed")
	}

	var msg botMessage
	if err := json.Unmarshal(result.Result, &msg); err != nil {
		return 0, fmt.Errorf("bot api parse message: %w", err)
	}
	return msg.MessageID, nil
}

func (b *BotAPIClient) DownloadPart(ctx context.Context, channelID int64, messageID int, w io.Writer) error {
	body := fmt.Sprintf(`{"chat_id":%s,"message_id":%d}`, b.chatID, messageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.botURL("getFile"), strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("bot api getFile: %w", err)
	}
	defer resp.Body.Close()

	var result botResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("bot api decode: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("bot api getFile failed for message %d", messageID)
	}

	var file botFile
	if err := json.Unmarshal(result.Result, &file); err != nil {
		return fmt.Errorf("bot api parse file: %w", err)
	}

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, b.fileURL(file.FilePath), nil)
	if err != nil {
		return err
	}

	dlResp, err := b.http.Do(dlReq)
	if err != nil {
		return fmt.Errorf("bot api download: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		return fmt.Errorf("bot api download status: %d", dlResp.StatusCode)
	}

	_, err = io.Copy(w, dlResp.Body)
	return err
}

func (b *BotAPIClient) DeleteMessages(ctx context.Context, channelID int64, messageIDs []int) error {
	for _, id := range messageIDs {
		body := fmt.Sprintf(`{"chat_id":%s,"message_id":%d}`, b.chatID, id)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.botURL("deleteMessage"), strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := b.http.Do(req)
		if err != nil {
			return fmt.Errorf("bot api deleteMessage %d: %w", id, err)
		}
		resp.Body.Close()
	}
	return nil
}

var _ domain.BlobStore = (*BotAPIClient)(nil)
