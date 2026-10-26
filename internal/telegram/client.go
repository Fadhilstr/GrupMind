package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Client adalah interface untuk berinteraksi dengan Telegram Bot API
type Client interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	SendMessageWithMode(ctx context.Context, chatID int64, text, parseMode string) error
	GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]Update, error)
	GetMe(ctx context.Context) (*User, error)
	IsChatAdmin(ctx context.Context, chatID, userID int64) (bool, error)
}

// HTTPClient mengimplementasikan Client menggunakan Telegram Bot HTTP API
type HTTPClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
	maxRetries int
}

// NewHTTPClient membuat instance HTTPClient baru
func NewHTTPClient(token string, logger *slog.Logger) *HTTPClient {
	return &HTTPClient{
		token:   token,
		baseURL: fmt.Sprintf("https://api.telegram.org/bot%s", token),
		httpClient: &http.Client{
			Timeout: 35 * time.Second,
		},
		logger:     logger,
		maxRetries: 3,
	}
}

// SetBaseURL mengubah base URL (sangat berguna untuk integration testing dengan httptest.Server)
func (c *HTTPClient) SetBaseURL(url string) {
	c.baseURL = url
}

// SendMessage mengirim pesan teks ke chat Telegram
func (c *HTTPClient) SendMessage(ctx context.Context, chatID int64, text string) error {
	return c.SendMessageWithMode(ctx, chatID, text, "")
}

// SendMessageWithMode mengirim pesan teks dengan parseMode (contoh: "HTML" atau "Markdown")
func (c *HTTPClient) SendMessageWithMode(ctx context.Context, chatID int64, text, parseMode string) error {
	reqPayload := SendMessageRequest{
		ChatID:    chatID,
		Text:      text,
		ParseMode: parseMode,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return fmt.Errorf("gagal serialize request sendMessage: %w", err)
	}

	endpoint := fmt.Sprintf("%s/sendMessage", c.baseURL)

	var lastErr error
	backoff := 500 * time.Millisecond

	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("gagal membuat request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("koneksi telegram gagal (percobaan %d/%d): %w", attempt, c.maxRetries, err)
			c.logger.Warn("Gagal mengirim pesan Telegram, mencoba kembali...", "attempt", attempt, "error", c.redact(err.Error()))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
				continue
			}
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("gagal membaca body respons telegram: %w", readErr)
			continue
		}

		var apiResp APIResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			lastErr = fmt.Errorf("gagal unmarshal respons telegram (status %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		if apiResp.OK {
			return nil
		}

		// Jika Telegram mengembalikan error API (misal 403 Forbidden, 400 Bad Request)
		lastErr = fmt.Errorf("telegram API error (kode %d): %s", apiResp.ErrorCode, apiResp.Description)

		// Retry jika rate limit (429) atau server error (>= 500)
		if apiResp.ErrorCode == 429 || apiResp.ErrorCode >= 500 {
			c.logger.Warn("Telegram API error sementara, mencoba kembali...", "code", apiResp.ErrorCode, "attempt", attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
				continue
			}
		}

		// Jika error client permanen (seperti 400 atau 403 bot di-block), jangan retry
		return lastErr
	}

	return lastErr
}

// GetUpdates mengambil antrean pesan baru melalui long-polling
func (c *HTTPClient) GetUpdates(ctx context.Context, offset int64, limit int, timeout int) ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&limit=%d&timeout=%d", c.baseURL, offset, limit, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request getUpdates: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi getUpdates: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca body getUpdates: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal unmarshal respons getUpdates: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("telegram getUpdates error (%d): %s", apiResp.ErrorCode, apiResp.Description)
	}

	var updates []Update
	if err := json.Unmarshal(apiResp.Result, &updates); err != nil {
		return nil, fmt.Errorf("gagal unmarshal daftar update: %w", err)
	}

	return updates, nil
}

// GetMe mengambil informasi akun bot
func (c *HTTPClient) GetMe(ctx context.Context) (*User, error) {
	url := fmt.Sprintf("%s/getMe", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("getMe error (%d): %s", apiResp.ErrorCode, apiResp.Description)
	}

	var user User
	if err := json.Unmarshal(apiResp.Result, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// IsChatAdmin memeriksa apakah userID tertentu adalah admin di grup chatID
func (c *HTTPClient) IsChatAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	url := fmt.Sprintf("%s/getChatMember?chat_id=%d&user_id=%d", c.baseURL, chatID, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return false, err
	}
	if !apiResp.OK {
		return false, fmt.Errorf("getChatMember error: %s", apiResp.Description)
	}

	var member struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(apiResp.Result, &member); err != nil {
		return false, err
	}

	return member.Status == "creator" || member.Status == "administrator", nil
}

// redact menyembunyikan token bot dari pesan log atau error
func (c *HTTPClient) redact(msg string) string {
	if c.token == "" {
		return msg
	}
	return strings.ReplaceAll(msg, c.token, "[REDACTED_TOKEN]")
}

