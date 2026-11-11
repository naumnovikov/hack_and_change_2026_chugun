package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Успешный GET запрос к /health",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "/health", nil)
			if err != nil {
				t.Fatalf("Не удалось создать запрос: %v", err)
			}

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(healthHandler)
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Ожидался статус %v, получен %v", tt.expectedStatus, status)
			}

			var resp HealthResponse
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Не удалось распарсить JSON ответ: %v", err)
			}

			if resp.Status != tt.expectedBody {
				t.Errorf("Ожидался статус в теле '%v', получен '%v'", tt.expectedBody, resp.Status)
			}
		})
	}
}
