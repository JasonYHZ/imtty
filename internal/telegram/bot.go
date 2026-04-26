package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"imtty/internal/stream"
)

type BotClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type TelegramFile struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
	FileSize int64  `json:"file_size,omitempty"`
}

func NewBotClient(baseURL string, token string, httpClient *http.Client) *BotClient {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}

	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &BotClient{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpClient,
	}
}

func (c *BotClient) SendMessage(ctx context.Context, chatID int64, message stream.OutboundMessage) error {
	statusCode, err := c.postMessage(ctx, chatID, message)
	if err != nil {
		return err
	}
	if statusCode >= 300 && message.ParseMode != "" {
		message.ParseMode = ""
		statusCode, err = c.postMessage(ctx, chatID, message)
		if err != nil {
			return err
		}
	}
	if statusCode >= 300 {
		return fmt.Errorf("telegram sendMessage returned status %d", statusCode)
	}

	return nil
}

func (c *BotClient) postMessage(ctx context.Context, chatID int64, message stream.OutboundMessage) (int, error) {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    message.Text,
	}
	if message.ParseMode != "" {
		payload["parse_mode"] = message.ParseMode
	}

	if len(message.QuickReplies) > 0 {
		keyboard := make([]map[string]string, 0, len(message.QuickReplies))
		for _, reply := range message.QuickReplies {
			keyboard = append(keyboard, map[string]string{"text": reply})
		}
		payload["reply_markup"] = map[string]any{
			"keyboard":          []any{keyboard},
			"resize_keyboard":   true,
			"one_time_keyboard": true,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+c.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	return response.StatusCode, nil
}

func (c *BotClient) SendChatAction(ctx context.Context, chatID int64, action string) error {
	payload := map[string]any{
		"chat_id": chatID,
		"action":  action,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+c.token+"/sendChatAction", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return fmt.Errorf("telegram sendChatAction returned status %d", response.StatusCode)
	}

	return nil
}

func (c *BotClient) SetMenuButton(ctx context.Context, text string, url string) error {
	payload := map[string]any{
		"menu_button": map[string]any{
			"type": "web_app",
			"text": text,
			"web_app": map[string]string{
				"url": url,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+c.token+"/setChatMenuButton", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return fmt.Errorf("telegram setChatMenuButton returned status %d", response.StatusCode)
	}

	return nil
}

func (c *BotClient) GetFile(ctx context.Context, fileID string) (TelegramFile, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/bot"+c.token+"/getFile?file_id="+url.QueryEscape(fileID), nil)
	if err != nil {
		return TelegramFile{}, err
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return TelegramFile{}, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return TelegramFile{}, fmt.Errorf("telegram getFile returned status %d", response.StatusCode)
	}

	var payload struct {
		OK     bool         `json:"ok"`
		Result TelegramFile `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return TelegramFile{}, err
	}
	if !payload.OK {
		return TelegramFile{}, fmt.Errorf("telegram getFile returned ok=false")
	}
	return payload.Result, nil
}

func (c *BotClient) DownloadFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/file/bot"+c.token+"/"+strings.TrimLeft(filePath, "/"), nil)
	if err != nil {
		return nil, err
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		defer response.Body.Close()
		return nil, fmt.Errorf("telegram file download returned status %d", response.StatusCode)
	}
	return response.Body, nil
}
