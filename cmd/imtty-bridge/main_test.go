package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewMuxRoutesMiniAppAssetSubpaths(t *testing.T) {
	webhookHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	miniAppHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(request.URL.Path))
	})

	mux := newMux(webhookHandler, miniAppHandler)

	request := httptest.NewRequest(http.MethodGet, "/mini-app/assets/index.js", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}
