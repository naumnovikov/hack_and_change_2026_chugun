package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

type HealthResponse struct {
	Status string `json:"status"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HealthResponse{Status: "ok"})
}

func main() {
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

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	log.Printf("🚀 Backend server запускается на порту %s...", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Критическая ошибка сервера: %v", err)
	}
}
