package telegram

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"imtty/internal/stream"
)

type webhookResponse struct {
	OK        bool     `json:"ok"`
	Responses []string `json:"responses,omitempty"`
}

type ReplySender interface {
	SendMessage(ctx context.Context, chatID int64, message stream.OutboundMessage) error
}

func NewWebhookHandler(secret string, adapter *Adapter, replySender ReplySender, logger *log.Logger) http.Handler {
	if logger == nil {
		logger = log.Default()
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "请求方法不被允许", http.StatusMethodNotAllowed)
			return
		}

		if request.Header.Get("X-Telegram-Bot-Api-Secret-Token") != secret {
			http.Error(writer, "鉴权失败", http.StatusUnauthorized)
			return
		}

		var update Update
		if err := json.NewDecoder(request.Body).Decode(&update); err != nil {
			http.Error(writer, "无效的 Telegram 更新请求", http.StatusBadRequest)
			return
		}

		responses := adapter.HandleUpdate(request.Context(), update)
		for _, response := range responses {
			logger.Printf("telegram outbound: %s", response)
			if update.Message != nil && replySender != nil {
				if err := replySender.SendMessage(request.Context(), update.Message.Chat.ID, stream.OutboundMessage{
					Text: response,
				}); err != nil {
					logger.Printf("telegram send reply: %v", err)
				}
			}
		}

		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(webhookResponse{
			OK:        true,
			Responses: responses,
		}); err != nil {
			logger.Printf("encode webhook response: %v", err)
		}
	})
}
