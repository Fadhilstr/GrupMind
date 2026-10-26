package telegram_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"botteleabsensi/internal/telegram"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClient_SendMessage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sendMessage", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req telegram.SendMessageRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, int64(-10012345), req.ChatID)
		assert.Equal(t, "Halo Dunia", req.Text)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(telegram.APIResponse{
			OK:     true,
			Result: json.RawMessage(`{"message_id": 123}`),
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := telegram.NewHTTPClient("dummy-token", logger)
	client.SetBaseURL(server.URL)

	err := client.SendMessage(context.Background(), -10012345, "Halo Dunia")
	require.NoError(t, err)
}

func TestHTTPClient_SendMessage_RetryOnServerError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(telegram.APIResponse{
				OK:          false,
				ErrorCode:   500,
				Description: "Internal Server Error",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(telegram.APIResponse{
			OK:     true,
			Result: json.RawMessage(`{"message_id": 456}`),
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := telegram.NewHTTPClient("dummy-token", logger)
	client.SetBaseURL(server.URL)

	err := client.SendMessage(context.Background(), -10012345, "Test Retry")
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestHTTPClient_SendMessage_ClientError_NoRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(telegram.APIResponse{
			OK:          false,
			ErrorCode:   403,
			Description: "Forbidden: bot was kicked from the supergroup chat",
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := telegram.NewHTTPClient("dummy-token", logger)
	client.SetBaseURL(server.URL)

	err := client.SendMessage(context.Background(), -10012345, "Test Kicked")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Forbidden")
	assert.Equal(t, 1, attempts, "Error 403 tidak boleh di-retry berulang kali")
}

func TestHTTPClient_GetUpdates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/getUpdates", r.URL.Path)
		assert.Equal(t, "10", r.URL.Query().Get("offset"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(telegram.APIResponse{
			OK: true,
			Result: json.RawMessage(`[
				{
					"update_id": 10,
					"message": {
						"message_id": 1,
						"date": 1600000000,
						"chat": {"id": -100111, "type": "supergroup"},
						"text": "/start"
					}
				}
			]`),
		})
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := telegram.NewHTTPClient("dummy-token", logger)
	client.SetBaseURL(server.URL)

	updates, err := client.GetUpdates(context.Background(), 10, 100, 30)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	assert.Equal(t, int64(10), updates[0].UpdateID)
	assert.Equal(t, "/start", updates[0].Message.Text)
}

