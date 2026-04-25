package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"imtty/internal/fileinput"
	"imtty/internal/session"
	"imtty/internal/stream"
)

func TestWebhookHandlerRejectsInvalidSecret(t *testing.T) {
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), &fakeSessionRuntime{}, &fakeProjectStore{}, nil, nil, nil)

	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))
	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-token")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestWebhookHandlerRoutesCommandsAndPlainText(t *testing.T) {
	runtime := &fakeSessionRuntime{}
	replies := &fakeReplySender{}
	store := &fakeProjectStore{}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, store, nil, nil, nil)
	handler := NewWebhookHandler("secret-token", adapter, replies, log.New(io.Discard, "", 0))

	response := sendTelegramUpdate(t, handler, "secret-token", `/list`)
	if !strings.Contains(response.Responses[0], "会话列表: 无") {
		t.Fatalf("list response = %#v, want empty session list", response.Responses)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/projects`)
	if !strings.Contains(response.Responses[0], "project-a => /tmp/project-a") {
		t.Fatalf("projects response = %#v, want project-a entry", response.Responses)
	}
	if got := replies.texts(42); len(got) < 2 || !strings.Contains(got[1], "project-a => /tmp/project-a") {
		t.Fatalf("reply sender texts = %#v, want project-a entry in /projects", got)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	if !strings.Contains(response.Responses[0], "已切换到会话 codex-project-a [running]") {
		t.Fatalf("open response = %#v, want chinese running codex-project-a", response.Responses)
	}
	if len(runtime.opened) != 1 || runtime.opened[0] != "42:codex-project-a" {
		t.Fatalf("runtime.opened = %#v, want 42:codex-project-a", runtime.opened)
	}

	response = sendTelegramUpdateAllowEmpty(t, handler, "secret-token", `hello from telegram`)
	if len(response.Responses) != 0 {
		t.Fatalf("text response = %#v, want no immediate ack for plain text", response.Responses)
	}

	if got := runtime.submitted["codex-project-a"]; len(got) != 1 || got[0] != "hello from telegram" {
		t.Fatalf("submitted[codex-project-a] = %#v, want hello from telegram", got)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/status`)
	if !strings.Contains(response.Responses[0], "当前会话: codex-project-a") {
		t.Fatalf("status response = %#v, want active session", response.Responses)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/close`)
	if !strings.Contains(response.Responses[0], "已关闭当前会话") {
		t.Fatalf("close response = %#v, want closed", response.Responses)
	}
	if len(runtime.closed) != 1 || runtime.closed[0] != "codex-project-a" {
		t.Fatalf("runtime.closed = %#v, want codex-project-a", runtime.closed)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `hello again`)
	if !strings.Contains(response.Responses[0], "请先执行 /open <project>") {
		t.Fatalf("no active response = %#v, want guidance", response.Responses)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	if !strings.Contains(response.Responses[0], "已切换到会话 codex-project-a [running]") {
		t.Fatalf("reopen response = %#v, want running codex-project-a", response.Responses)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/kill`)
	if !strings.Contains(response.Responses[0], "已彻底删除会话 codex-project-a") {
		t.Fatalf("kill response = %#v, want delete wording", response.Responses)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/list`)
	if !strings.Contains(response.Responses[0], "会话列表: 无") || strings.Contains(response.Responses[0], "project-a => /tmp/project-a") {
		t.Fatalf("list response = %#v, want empty sessions only", response.Responses)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/project_add project-b /tmp/project-b`)
	if !strings.Contains(response.Responses[0], "已添加项目 project-b") {
		t.Fatalf("project-add response = %#v, want added project-b", response.Responses)
	}
	if got := store.added["project-b"]; got != "/tmp/project-b" {
		t.Fatalf("store.added[project-b] = %q, want %q", got, "/tmp/project-b")
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/projects`)
	if !strings.Contains(response.Responses[0], "project-b => /tmp/project-b") {
		t.Fatalf("projects response = %#v, want project-b entry", response.Responses)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/project_remove project-b`)
	if !strings.Contains(response.Responses[0], "已移除项目 project-b") {
		t.Fatalf("project-remove response = %#v, want removed project-b", response.Responses)
	}
	if len(store.removed) != 1 || store.removed[0] != "project-b" {
		t.Fatalf("store.removed = %#v, want project-b", store.removed)
	}
	if len(runtime.killed) != 1 || runtime.killed[0] != "codex-project-a" {
		t.Fatalf("runtime.killed = %#v, want only codex-project-a from /kill", runtime.killed)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/open project-b`)
	if !strings.Contains(response.Responses[0], "打开项目失败") {
		t.Fatalf("open removed project response = %#v, want failure", response.Responses)
	}

	response = sendTelegramUpdate(t, handler, "secret-token", `/project_remove project-a`)
	if !strings.Contains(response.Responses[0], "移除项目失败") {
		t.Fatalf("remove static project response = %#v, want failure", response.Responses)
	}
}

func TestWebhookHandlerReportsOpenFailureWhenTmuxSetupFails(t *testing.T) {
	runtime := &fakeSessionRuntime{
		openErr: io.ErrUnexpectedEOF,
	}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, nil, nil, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	response := sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	if !strings.Contains(response.Responses[0], "打开项目失败") {
		t.Fatalf("open response = %#v, want chinese open failed", response.Responses)
	}
}

func TestWebhookHandlerReportsRuntimeOpenFailureAfterOpen(t *testing.T) {
	runtime := &fakeSessionRuntime{openErr: io.ErrNoProgress}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, nil, nil, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	response := sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	if len(response.Responses) != 1 || !strings.Contains(response.Responses[0], "打开项目失败") {
		t.Fatalf("open response = %#v, want pump failure", response.Responses)
	}
}

func TestWebhookHandlerAllowsEmptyImmediateResponsesForPlainText(t *testing.T) {
	runtime := &fakeSessionRuntime{}
	replies := &fakeReplySender{}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, nil, nil, nil)
	handler := NewWebhookHandler("secret-token", adapter, replies, log.New(io.Discard, "", 0))

	openResponse := sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	if len(openResponse.Responses) != 1 {
		t.Fatalf("open response = %#v, want one command response", openResponse.Responses)
	}

	response := sendTelegramUpdateAllowEmpty(t, handler, "secret-token", `hi`)
	if len(response.Responses) != 0 {
		t.Fatalf("plain text response = %#v, want empty immediate response", response.Responses)
	}
	if got := replies.texts(42); len(got) != 1 || !strings.Contains(got[0], "已切换到会话 codex-project-a") {
		t.Fatalf("reply sender texts = %#v, want only /open reply", got)
	}
}

func TestWebhookHandlerRejectsPlainTextWhenSessionLocallyAttached(t *testing.T) {
	runtime := &fakeSessionRuntime{
		locallyAttached: map[string]bool{"codex-project-a": true},
	}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, nil, nil, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramUpdate(t, handler, "secret-token", `hello`)
	if !strings.Contains(response.Responses[0], "本地占用中") {
		t.Fatalf("response = %#v, want local attached guidance", response.Responses)
	}
}

func TestWebhookHandlerAllowsPlainTextWhenSessionIsReadonlySpectatorOnly(t *testing.T) {
	runtime := &fakeSessionRuntime{
		locallyAttached: map[string]bool{"codex-project-a": false},
	}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, nil, nil, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramUpdateAllowEmpty(t, handler, "secret-token", `hello`)
	if len(response.Responses) != 0 {
		t.Fatalf("response = %#v, want readonly spectator to allow silent submission", response.Responses)
	}
	if got := runtime.submitted["codex-project-a"]; len(got) != 1 || got[0] != "hello" {
		t.Fatalf("submitted = %#v, want hello", runtime.submitted)
	}
}

func TestWebhookHandlerRoutesApprovalReplyToPendingRequest(t *testing.T) {
	runtime := &fakeSessionRuntime{
		pendingApproval: true,
	}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, nil, nil, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramUpdateAllowEmpty(t, handler, "secret-token", `是`)
	if len(response.Responses) != 0 {
		t.Fatalf("response = %#v, want no immediate ack for approval reply", response.Responses)
	}
	if len(runtime.approvals["codex-project-a"]) != 1 || runtime.approvals["codex-project-a"][0] != "是" {
		t.Fatalf("approvals = %#v, want approval reply", runtime.approvals)
	}
}

func TestWebhookHandlerRejectsKillWhenSessionLocallyAttached(t *testing.T) {
	runtime := &fakeSessionRuntime{
		locallyAttached: map[string]bool{"codex-project-a": true},
	}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, nil, nil, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramUpdate(t, handler, "secret-token", `/kill`)
	if !strings.Contains(response.Responses[0], "本地占用中") {
		t.Fatalf("response = %#v, want local attached guidance", response.Responses)
	}
}

func TestWebhookHandlerPhotoMessageSubmitsImageTurn(t *testing.T) {
	runtime := &fakeSessionRuntime{}
	fileClient := &fakeFileClient{
		files: map[string]TelegramFile{
			"photo-large": {FileID: "photo-large", FilePath: "photos/photo-large.jpg"},
		},
		content: map[string]string{
			"photos/photo-large.jpg": "jpeg-bytes",
		},
	}
	mediaStore := &fakeMediaStore{path: "/tmp/imtty-media/codex-project-a/photo-large.jpg"}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, fileClient, mediaStore, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramPhotoUpdateAllowEmpty(t, handler, "secret-token", []PhotoSize{
		{FileID: "photo-small", FileSize: 10},
		{FileID: "photo-large", FileSize: 20},
	}, "看看这张图")
	if len(response.Responses) != 0 {
		t.Fatalf("response = %#v, want no immediate ack", response.Responses)
	}
	if len(fileClient.getFileIDs) != 1 || fileClient.getFileIDs[0] != "photo-large" {
		t.Fatalf("getFileIDs = %#v, want photo-large", fileClient.getFileIDs)
	}
	if mediaStore.savedExtension != ".jpg" {
		t.Fatalf("savedExtension = %q, want .jpg", mediaStore.savedExtension)
	}
	if got := runtime.images["codex-project-a"]; len(got) != 1 || got[0].path != "/tmp/imtty-media/codex-project-a/photo-large.jpg" || got[0].caption != "看看这张图" {
		t.Fatalf("images = %#v, want saved image with caption", runtime.images)
	}
}

func TestWebhookHandlerImageDocumentSubmitsImageTurn(t *testing.T) {
	runtime := &fakeSessionRuntime{}
	fileClient := &fakeFileClient{
		files: map[string]TelegramFile{
			"doc-image": {FileID: "doc-image", FilePath: "documents/diagram.png"},
		},
		content: map[string]string{
			"documents/diagram.png": "png-bytes",
		},
	}
	mediaStore := &fakeMediaStore{path: "/tmp/imtty-media/codex-project-a/diagram.png"}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, fileClient, mediaStore, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramDocumentUpdateAllowEmpty(t, handler, "secret-token", Document{
		FileID:   "doc-image",
		FileName: "diagram.png",
		MimeType: "image/png",
	}, "解释一下")
	if len(response.Responses) != 0 {
		t.Fatalf("response = %#v, want no immediate ack", response.Responses)
	}
	if mediaStore.savedExtension != ".png" {
		t.Fatalf("savedExtension = %q, want .png", mediaStore.savedExtension)
	}
	if got := runtime.images["codex-project-a"]; len(got) != 1 || got[0].caption != "解释一下" {
		t.Fatalf("images = %#v, want document image with caption", runtime.images)
	}
}

func TestWebhookHandlerTextDocumentSubmitsTextTurn(t *testing.T) {
	runtime := &fakeSessionRuntime{}
	fileClient := &fakeFileClient{
		files: map[string]TelegramFile{
			"doc-text": {FileID: "doc-text", FilePath: "documents/notes.md"},
		},
		content: map[string]string{
			"documents/notes.md": "# heading\nhello",
		},
	}
	mediaStore := &fakeMediaStore{path: "/tmp/imtty-media/codex-project-a/notes.md"}
	analyzer := &fakeDocumentAnalyzer{text: "分析输入文本"}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, fileClient, mediaStore, analyzer)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramDocumentUpdateAllowEmpty(t, handler, "secret-token", Document{
		FileID:   "doc-text",
		FileName: "notes.md",
		MimeType: "text/markdown",
	}, "帮我总结")
	if len(response.Responses) != 0 {
		t.Fatalf("response = %#v, want no immediate ack", response.Responses)
	}
	if mediaStore.savedExtension != ".md" {
		t.Fatalf("savedExtension = %q, want .md", mediaStore.savedExtension)
	}
	if len(analyzer.calls) != 1 || analyzer.calls[0].path != "/tmp/imtty-media/codex-project-a/notes.md" {
		t.Fatalf("calls = %#v, want saved path", analyzer.calls)
	}
	if got := runtime.submitted["codex-project-a"]; len(got) != 1 || got[0] != "分析输入文本" {
		t.Fatalf("submitted = %#v, want analyzed text", runtime.submitted)
	}
}

func TestWebhookHandlerPDFDocumentSubmitsExtractedTextTurn(t *testing.T) {
	runtime := &fakeSessionRuntime{}
	fileClient := &fakeFileClient{
		files: map[string]TelegramFile{
			"doc-pdf": {FileID: "doc-pdf", FilePath: "documents/paper.pdf"},
		},
		content: map[string]string{
			"documents/paper.pdf": "%PDF",
		},
	}
	mediaStore := &fakeMediaStore{path: "/tmp/imtty-media/codex-project-a/paper.pdf"}
	analyzer := &fakeDocumentAnalyzer{text: "提取后的 PDF 文本"}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, fileClient, mediaStore, analyzer)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramDocumentUpdateAllowEmpty(t, handler, "secret-token", Document{
		FileID:   "doc-pdf",
		FileName: "paper.pdf",
		MimeType: "application/pdf",
	}, "提炼重点")
	if len(response.Responses) != 0 {
		t.Fatalf("response = %#v, want no immediate ack", response.Responses)
	}
	if got := runtime.submitted["codex-project-a"]; len(got) != 1 || got[0] != "提取后的 PDF 文本" {
		t.Fatalf("submitted = %#v, want extracted pdf text", runtime.submitted)
	}
}

func TestWebhookHandlerRejectsNonImageDocument(t *testing.T) {
	runtime := &fakeSessionRuntime{}
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), runtime, &fakeProjectStore{}, &fakeFileClient{
		files: map[string]TelegramFile{
			"doc-1": {FileID: "doc-1", FilePath: "documents/notes.txt"},
		},
		content: map[string]string{
			"documents/notes.txt": "hello",
		},
	}, &fakeMediaStore{path: "/tmp/imtty-media/codex-project-a/notes.txt"}, &fakeDocumentAnalyzer{text: "文本文件"})
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramDocumentUpdateAllowEmpty(t, handler, "secret-token", Document{
		FileID:   "doc-1",
		FileName: "notes.txt",
		MimeType: "text/plain",
	}, "")
	if len(response.Responses) != 0 {
		t.Fatalf("response = %#v, want text document accepted", response.Responses)
	}
}

func TestWebhookHandlerRejectsUnsupportedBinaryDocument(t *testing.T) {
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), &fakeSessionRuntime{}, &fakeProjectStore{}, &fakeFileClient{}, &fakeMediaStore{}, &fakeDocumentAnalyzer{err: fileinput.ErrUnsupportedFileType})
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	sendTelegramUpdate(t, handler, "secret-token", `/open project-a`)
	response := sendTelegramDocumentUpdateAllowEmpty(t, handler, "secret-token", Document{
		FileID:   "doc-bin",
		FileName: "archive.zip",
		MimeType: "application/zip",
	}, "")
	if len(response.Responses) != 1 || !strings.Contains(response.Responses[0], "当前只支持图片、文本文件和 PDF") {
		t.Fatalf("response = %#v, want unsupported binary rejection", response.Responses)
	}
}

func TestWebhookHandlerImageMessageRequiresActiveSession(t *testing.T) {
	adapter := NewAdapter(session.NewRegistry(map[string]string{
		"project-a": "/tmp/project-a",
	}), &fakeSessionRuntime{}, &fakeProjectStore{}, &fakeFileClient{}, &fakeMediaStore{}, nil)
	handler := NewWebhookHandler("secret-token", adapter, &fakeReplySender{}, log.New(io.Discard, "", 0))

	response := sendTelegramPhotoUpdateAllowEmpty(t, handler, "secret-token", []PhotoSize{
		{FileID: "photo-large", FileSize: 20},
	}, "")
	if len(response.Responses) != 1 || !strings.Contains(response.Responses[0], "请先执行 /open <project>") {
		t.Fatalf("response = %#v, want active session guidance", response.Responses)
	}
}

type handlerResponse struct {
	OK        bool     `json:"ok"`
	Responses []string `json:"responses"`
}

func sendTelegramUpdate(t *testing.T, handler http.Handler, secret string, text string) handlerResponse {
	t.Helper()
	response := sendTelegramUpdateAllowEmpty(t, handler, secret, text)
	if len(response.Responses) == 0 {
		t.Fatalf("response.Responses = %#v, want at least one response", response.Responses)
	}
	return response
}

func sendTelegramUpdateAllowEmpty(t *testing.T, handler http.Handler, secret string, text string) handlerResponse {
	t.Helper()

	body, err := json.Marshal(Update{
		UpdateID: 1,
		Message: &Message{
			MessageID: 1,
			Chat: Chat{
				ID: 42,
			},
			From: &User{
				ID:       7,
				Username: "tester",
			},
			Text: text,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response handlerResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if !response.OK {
		t.Fatalf("response.OK = false, want true")
	}

	return response
}

type fakeSessionRuntime struct {
	opened          []string
	closed          []string
	killed          []string
	submitted       map[string][]string
	images          map[string][]submittedImage
	approvals       map[string][]string
	locallyAttached map[string]bool
	pendingApproval bool
	openErr         error
	submitErr       error
	approvalErr     error
	killErr         error
}

func (f *fakeSessionRuntime) OpenSession(_ context.Context, chatID int64, view session.View) error {
	if f.openErr != nil {
		return f.openErr
	}
	f.opened = append(f.opened, formatRuntimeStart(chatID, view.Name))
	return nil
}

func (f *fakeSessionRuntime) CloseSession(sessionName string) {
	f.closed = append(f.closed, sessionName)
}

func (f *fakeSessionRuntime) SubmitText(_ context.Context, _ int64, view session.View, text string) error {
	if f.submitErr != nil {
		return f.submitErr
	}
	if f.submitted == nil {
		f.submitted = make(map[string][]string)
	}
	f.submitted[view.Name] = append(f.submitted[view.Name], text)
	return nil
}

type submittedImage struct {
	path    string
	caption string
}

func (f *fakeSessionRuntime) SubmitImage(_ context.Context, _ int64, view session.View, imagePath string, caption string) error {
	if f.submitErr != nil {
		return f.submitErr
	}
	if f.images == nil {
		f.images = make(map[string][]submittedImage)
	}
	f.images[view.Name] = append(f.images[view.Name], submittedImage{path: imagePath, caption: caption})
	return nil
}

func (f *fakeSessionRuntime) SubmitApproval(_ context.Context, _ int64, view session.View, text string) (bool, error) {
	if !f.pendingApproval {
		return false, nil
	}
	if f.approvalErr != nil {
		return true, f.approvalErr
	}
	if f.approvals == nil {
		f.approvals = make(map[string][]string)
	}
	f.approvals[view.Name] = append(f.approvals[view.Name], text)
	return true, nil
}

func (f *fakeSessionRuntime) IsLocallyAttached(_ context.Context, sessionName string) (bool, error) {
	return f.locallyAttached[sessionName], nil
}

func (f *fakeSessionRuntime) KillSession(_ context.Context, view session.View) error {
	if f.killErr != nil {
		return f.killErr
	}
	f.killed = append(f.killed, view.Name)
	return nil
}

func formatRuntimeStart(chatID int64, sessionName string) string {
	return strconv.FormatInt(chatID, 10) + ":" + sessionName
}

type fakeReplySender struct {
	outbound map[int64][]string
}

func (f *fakeReplySender) SendMessage(_ context.Context, chatID int64, message stream.OutboundMessage) error {
	if f.outbound == nil {
		f.outbound = make(map[int64][]string)
	}
	f.outbound[chatID] = append(f.outbound[chatID], message.Text)
	return nil
}

func (f *fakeReplySender) texts(chatID int64) []string {
	return f.outbound[chatID]
}

type fakeProjectStore struct {
	added     map[string]string
	removed   []string
	addErr    error
	removeErr error
}

func (f *fakeProjectStore) AddProject(name string, root string) error {
	if f.addErr != nil {
		return f.addErr
	}
	if f.added == nil {
		f.added = make(map[string]string)
	}
	f.added[name] = root
	return nil
}

func (f *fakeProjectStore) RemoveProject(name string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, name)
	return nil
}

type fakeFileClient struct {
	files      map[string]TelegramFile
	content    map[string]string
	getFileIDs []string
}

func (f *fakeFileClient) GetFile(_ context.Context, fileID string) (TelegramFile, error) {
	f.getFileIDs = append(f.getFileIDs, fileID)
	return f.files[fileID], nil
}

func (f *fakeFileClient) DownloadFile(_ context.Context, filePath string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.content[filePath])), nil
}

type fakeMediaStore struct {
	path           string
	savedSession   string
	savedFileID    string
	savedExtension string
	savedContent   string
}

func (f *fakeMediaStore) SaveImage(sessionName string, fileID string, extension string, body io.Reader) (string, error) {
	content, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	f.savedSession = sessionName
	f.savedFileID = fileID
	f.savedExtension = extension
	f.savedContent = string(content)
	return f.path, nil
}

func (f *fakeMediaStore) SaveDocument(sessionName string, fileID string, extension string, body io.Reader) (string, error) {
	return f.SaveImage(sessionName, fileID, extension, body)
}

type fakeDocumentAnalyzer struct {
	text  string
	err   error
	calls []documentAnalyzerCall
}

type documentAnalyzerCall struct {
	path     string
	fileName string
	mimeType string
	caption  string
}

func (f *fakeDocumentAnalyzer) BuildTurnText(_ context.Context, path string, fileName string, mimeType string, caption string) (string, error) {
	f.calls = append(f.calls, documentAnalyzerCall{
		path:     path,
		fileName: fileName,
		mimeType: mimeType,
		caption:  caption,
	})
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}

func sendTelegramPhotoUpdateAllowEmpty(t *testing.T, handler http.Handler, secret string, photos []PhotoSize, caption string) handlerResponse {
	t.Helper()
	return sendTelegramMessage(t, handler, secret, Message{
		MessageID: 1,
		Chat:      Chat{ID: 42},
		From:      &User{ID: 7, Username: "tester"},
		Caption:   caption,
		Photo:     photos,
	})
}

func sendTelegramDocumentUpdateAllowEmpty(t *testing.T, handler http.Handler, secret string, document Document, caption string) handlerResponse {
	t.Helper()
	return sendTelegramMessage(t, handler, secret, Message{
		MessageID: 1,
		Chat:      Chat{ID: 42},
		From:      &User{ID: 7, Username: "tester"},
		Caption:   caption,
		Document:  &document,
	})
}

func sendTelegramMessage(t *testing.T, handler http.Handler, secret string, message Message) handlerResponse {
	t.Helper()

	body, err := json.Marshal(Update{
		UpdateID: 1,
		Message:  &message,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/telegram/webhook", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response handlerResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !response.OK {
		t.Fatalf("response.OK = false, want true")
	}
	return response
}
