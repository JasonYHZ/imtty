package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"imtty/internal/stream"
)

func TestBotClientSendMessagePostsToTelegramAPI(t *testing.T) {
	type requestBody struct {
		ChatID      int64  `json:"chat_id"`
		Text        string `json:"text"`
		ReplyMarkup struct {
			Keyboard [][]struct {
				Text string `json:"text"`
			} `json:"keyboard"`
			ResizeKeyboard  bool `json:"resize_keyboard"`
			OneTimeKeyboard bool `json:"one_time_keyboard"`
		} `json:"reply_markup"`
	}

	var (
		gotPath string
		gotBody requestBody
	)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotPath = request.URL.Path
		if err := json.NewDecoder(request.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewBotClient(server.URL, "bot-token", server.Client())
	err := client.SendMessage(context.Background(), 42, stream.OutboundMessage{
		Text:         "hello from pump",
		QuickReplies: []string{"是", "否"},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotPath != "/botbot-token/sendMessage" {
		t.Fatalf("path = %q, want %q", gotPath, "/botbot-token/sendMessage")
	}

	if gotBody.ChatID != 42 || gotBody.Text != "hello from pump" {
		t.Fatalf("body = %#v, want chat_id=42 text=hello from pump", gotBody)
	}

	if len(gotBody.ReplyMarkup.Keyboard) != 1 || len(gotBody.ReplyMarkup.Keyboard[0]) != 2 {
		t.Fatalf("reply_markup.keyboard = %#v, want one row with two buttons", gotBody.ReplyMarkup.Keyboard)
	}

	if gotBody.ReplyMarkup.Keyboard[0][0].Text != "是" || gotBody.ReplyMarkup.Keyboard[0][1].Text != "否" {
		t.Fatalf("reply_markup.keyboard = %#v, want 是/否", gotBody.ReplyMarkup.Keyboard)
	}
}

func TestBotClientSetMenuButtonPostsWebAppButton(t *testing.T) {
	type requestBody struct {
		MenuButton struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			WebApp struct {
				URL string `json:"url"`
			} `json:"web_app"`
		} `json:"menu_button"`
	}

	var (
		gotPath string
		gotBody requestBody
	)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotPath = request.URL.Path
		if err := json.NewDecoder(request.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewBotClient(server.URL, "bot-token", server.Client())
	if err := client.SetMenuButton(context.Background(), "控制面板", "https://example.com/mini-app"); err != nil {
		t.Fatalf("SetMenuButton() error = %v", err)
	}

	if gotPath != "/botbot-token/setChatMenuButton" {
		t.Fatalf("path = %q, want %q", gotPath, "/botbot-token/setChatMenuButton")
	}

	if gotBody.MenuButton.Type != "web_app" {
		t.Fatalf("menu_button.type = %q, want %q", gotBody.MenuButton.Type, "web_app")
	}

	if gotBody.MenuButton.Text != "控制面板" {
		t.Fatalf("menu_button.text = %q, want %q", gotBody.MenuButton.Text, "控制面板")
	}

	if gotBody.MenuButton.WebApp.URL != "https://example.com/mini-app" {
		t.Fatalf("menu_button.web_app.url = %q, want %q", gotBody.MenuButton.WebApp.URL, "https://example.com/mini-app")
	}
}

func TestBotClientSendChatActionPostsTypingAction(t *testing.T) {
	type requestBody struct {
		ChatID int64  `json:"chat_id"`
		Action string `json:"action"`
	}

	var (
		gotPath string
		gotBody requestBody
	)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotPath = request.URL.Path
		if err := json.NewDecoder(request.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewBotClient(server.URL, "bot-token", server.Client())
	if err := client.SendChatAction(context.Background(), 42, "typing"); err != nil {
		t.Fatalf("SendChatAction() error = %v", err)
	}

	if gotPath != "/botbot-token/sendChatAction" {
		t.Fatalf("path = %q, want %q", gotPath, "/botbot-token/sendChatAction")
	}
	if gotBody.ChatID != 42 || gotBody.Action != "typing" {
		t.Fatalf("body = %#v, want chat_id=42 action=typing", gotBody)
	}
}

func TestBotClientGetFileReadsTelegramFilePath(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotPath = request.URL.Path + "?" + request.URL.RawQuery
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":{"file_id":"file-1","file_path":"photos/file-1.jpg","file_size":123}}`))
	}))
	defer server.Close()

	client := NewBotClient(server.URL, "bot-token", server.Client())
	file, err := client.GetFile(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("GetFile() error = %v", err)
	}

	if gotPath != "/botbot-token/getFile?file_id=file-1" {
		t.Fatalf("path = %q, want %q", gotPath, "/botbot-token/getFile?file_id=file-1")
	}
	if file.FilePath != "photos/file-1.jpg" {
		t.Fatalf("FilePath = %q, want %q", file.FilePath, "photos/file-1.jpg")
	}
}

func TestBotClientDownloadFileFetchesBinaryBody(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotPath = request.URL.Path
		_, _ = writer.Write([]byte("jpeg-bytes"))
	}))
	defer server.Close()

	client := NewBotClient(server.URL, "bot-token", server.Client())
	body, err := client.DownloadFile(context.Background(), "photos/file-1.jpg")
	if err != nil {
		t.Fatalf("DownloadFile() error = %v", err)
	}
	defer body.Close()

	if gotPath != "/file/botbot-token/photos/file-1.jpg" {
		t.Fatalf("path = %q, want %q", gotPath, "/file/botbot-token/photos/file-1.jpg")
	}

	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(content) != "jpeg-bytes" {
		t.Fatalf("content = %q, want %q", string(content), "jpeg-bytes")
	}
}
