package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type HealthResponse struct {
	Status string `json:"status"`
}

type AccessRequestPayload struct {
	ServiceName string `json:"service_name"`
}

type AccessRequestResponse struct {
	TicketID string `json:"ticket_id"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}

type SendMessagePayload struct {
	RecipientID string `json:"recipient_id"`
	Text        string `json:"text"`
}

type SendMessageResponse struct {
	MessageID string `json:"message_id"`
	SentAt    string `json:"sent_at"`
}

type EmployeeResponse struct {
	EmployeeID   string `json:"employee_id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	MattermostID string `json:"mattermost_id"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func createAccessRequestHandler(w http.ResponseWriter, r *http.Request) {
	var payload AccessRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.ServiceName) == "" {
		apiError(w, http.StatusBadRequest, "invalid_request", "service_name is required")
		return
	}

	resp := AccessRequestResponse{
		TicketID: "TICKET-1024",
		Status:   "created",
		Message:  fmt.Sprintf("Заявка на доступ к %s успешно создана", payload.ServiceName),
	}
	writeJSON(w, http.StatusOK, resp)
}

func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	var payload SendMessagePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil ||
		strings.TrimSpace(payload.RecipientID) == "" ||
		strings.TrimSpace(payload.Text) == "" {
		apiError(w, http.StatusBadRequest, "invalid_request", "recipient_id and text are required")
		return
	}

	resp := SendMessageResponse{
		MessageID: "MSG-2048",
		SentAt:    time.Now().UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

func getEmployeeByServiceHandler(w http.ResponseWriter, r *http.Request) {
	serviceName := strings.TrimSpace(r.URL.Query().Get("service_name"))
	if serviceName == "" {
		apiError(w, http.StatusBadRequest, "invalid_request", "service_name query param is required")
		return
	}

	resp := EmployeeResponse{
		EmployeeID:   "emp-001",
		Name:         "Иван Петров",
		Role:         fmt.Sprintf("Tech Lead (%s)", serviceName),
		MattermostID: "@ipetrov",
	}
	writeJSON(w, http.StatusOK, resp)
}

func getEmployeeByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		apiError(w, http.StatusBadRequest, "invalid_request", "id path param is required")
		return
	}

	resp := EmployeeResponse{
		EmployeeID:   id,
		Name:         "Иван Петров",
		Role:         "Senior Backend Engineer",
		MattermostID: "@ipetrov",
	}
	writeJSON(w, http.StatusOK, resp)
}

func setupRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /api/chat", chatHandler(&http.Client{Timeout: 30 * time.Second}, os.Getenv("AI_SERVICE_URL")))
	mux.HandleFunc("POST /api/mock/access-request", createAccessRequestHandler)
	mux.HandleFunc("POST /api/mock/message", sendMessageHandler)
	mux.HandleFunc("GET /api/mock/employee", getEmployeeByServiceHandler)
	mux.HandleFunc("GET /api/mock/employee/{id}", getEmployeeByIDHandler)
	return mux
}

func loadConfig() string {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Println("WARN: DATABASE_URL не задан, работаем без БД")
	} else {
		log.Println("INFO: Строка подключения к БД получена")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return port
}

func main() {
	port := loadConfig()
	store, err := openStore(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.DB.Close()
	mux := persistentActions(store)
	mux.HandleFunc("POST /api/chat", chatHandler(&http.Client{Timeout: 30 * time.Second}, os.Getenv("AI_SERVICE_URL")))
	log.Printf("🚀 Backend server запускается на порту %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
