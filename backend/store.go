package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PostgresStore persists successful mock actions; no real corporate services are called.
type PostgresStore struct{ DB *sql.DB }

const mockSchema = `CREATE TABLE IF NOT EXISTS mock_access_requests (
 id BIGSERIAL PRIMARY KEY, service_name TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'created', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS mock_messages (
 id BIGSERIAL PRIMARY KEY, recipient_id TEXT NOT NULL, text TEXT NOT NULL, sent_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

func (s PostgresStore) Initialize(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, mockSchema+demoSchema)
	return err
}
func (s PostgresStore) CreateAccess(ctx context.Context, service string) (AccessRequestResponse, error) {
	var id int64
	var status string
	err := s.DB.QueryRowContext(ctx, "INSERT INTO mock_access_requests (service_name) VALUES ($1) RETURNING id, status", service).Scan(&id, &status)
	if err != nil {
		return AccessRequestResponse{}, err
	}
	return AccessRequestResponse{TicketID: fmt.Sprintf("DEMO-ACCESS-%d", id), Status: status, Message: fmt.Sprintf("Демо-заявка на доступ к %s создана", service)}, nil
}
func (s PostgresStore) SendMessage(ctx context.Context, recipient, text string) (SendMessageResponse, error) {
	var id int64
	var sentAt time.Time
	err := s.DB.QueryRowContext(ctx, "INSERT INTO mock_messages (recipient_id, text) VALUES ($1,$2) RETURNING id, sent_at", recipient, text).Scan(&id, &sentAt)
	if err != nil {
		return SendMessageResponse{}, err
	}
	return SendMessageResponse{MessageID: fmt.Sprintf("DEMO-MSG-%d", id), SentAt: sentAt.UTC().Format(time.RFC3339Nano)}, nil
}

type actionStore interface {
	CreateAccess(context.Context, string) (AccessRequestResponse, error)
	SendMessage(context.Context, string, string) (SendMessageResponse, error)
}

func persistentActions(store actionStore) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /api/mock/employee", getEmployeeByServiceHandler)
	mux.HandleFunc("GET /api/mock/employee/{id}", getEmployeeByIDHandler)
	mux.HandleFunc("POST /api/mock/access-request", func(w http.ResponseWriter, r *http.Request) {
		var input AccessRequestPayload
		if decodeBody(w, r, &input) != nil || strings.TrimSpace(input.ServiceName) == "" {
			apiError(w, 400, "invalid_request", "service_name is required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := store.CreateAccess(ctx, input.ServiceName)
		if err != nil {
			apiError(w, 503, "storage_unavailable", "Не удалось сохранить демо-заявку")
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /api/mock/message", func(w http.ResponseWriter, r *http.Request) {
		var input SendMessagePayload
		if decodeBody(w, r, &input) != nil || strings.TrimSpace(input.RecipientID) == "" || strings.TrimSpace(input.Text) == "" {
			apiError(w, 400, "invalid_request", "recipient_id and text are required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := store.SendMessage(ctx, input.RecipientID, input.Text)
		if err != nil {
			apiError(w, 503, "storage_unavailable", "Не удалось сохранить демо-сообщение")
			return
		}
		writeJSON(w, 200, result)
	})
	return mux
}
