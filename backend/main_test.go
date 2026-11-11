package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Run("Переменные окружения не заданы (дефолты)", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		t.Setenv("PORT", "")

		port := loadConfig()
		if port != "8080" {
			t.Errorf("ожидался дефолтный порт 8080, получен %s", port)
		}
	})

	t.Run("Переменные окружения заданы", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://avito:avito@postgres:5432/aibuddy?sslmode=disable")
		t.Setenv("PORT", "9090")

		port := loadConfig()
		if port != "9090" {
			t.Errorf("ожидался порт 9090, получен %s", port)
		}
	})
}

func TestAPI(t *testing.T) {
	mux := setupRouter()

	t.Run("GET /health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("ожидался статус 200, получен %d", rr.Code)
		}
		var resp HealthResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil || resp.Status != "ok" {
			t.Errorf("некорректный ответ: %+v, err: %v", resp, err)
		}
	})

	t.Run("POST /api/mock/access-request (успех и ошибка валидации)", func(t *testing.T) {
		body := bytes.NewBufferString(`{"employee_id":"emp-001","service_name":"Jira"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/mock/access-request", body)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("ожидался статус 200, получен %d", rr.Code)
		}
		var resp AccessRequestResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil || resp.TicketID == "" || resp.Status == "" {
			t.Errorf("некорректный ответ: %+v, err: %v", resp, err)
		}

		badReq := httptest.NewRequest(http.MethodPost, "/api/mock/access-request", bytes.NewBufferString(`{}`))
		badRR := httptest.NewRecorder()
		mux.ServeHTTP(badRR, badReq)
		if badRR.Code != http.StatusBadRequest {
			t.Errorf("ожидался статус 400, получен %d", badRR.Code)
		}
	})

	t.Run("POST /api/mock/message (успех и ошибка валидации)", func(t *testing.T) {
		body := bytes.NewBufferString(`{"sender_id":"emp-002","recipient_id":"emp-001","text":"Привет, кто на кнопке?"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/mock/message", body)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("ожидался статус 200, получен %d", rr.Code)
		}
		var resp SendMessageResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil || resp.MessageID == "" || resp.SentAt == "" {
			t.Errorf("некорректный ответ: %+v, err: %v", resp, err)
		}

		badReq := httptest.NewRequest(http.MethodPost, "/api/mock/message", bytes.NewBufferString(`{"recipient_id":"emp-001"}`))
		badRR := httptest.NewRecorder()
		mux.ServeHTTP(badRR, badReq)
		if badRR.Code != http.StatusBadRequest {
			t.Errorf("ожидался статус 400, получен %d", badRR.Code)
		}
	})

	t.Run("GET /api/mock/employee (поиск по service_name)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/mock/employee?service_name=billing", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("ожидался статус 200, получен %d", rr.Code)
		}
		var resp EmployeeResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil || resp.EmployeeID == "" || resp.MattermostID == "" {
			t.Errorf("некорректный ответ: %+v, err: %v", resp, err)
		}

		badReq := httptest.NewRequest(http.MethodGet, "/api/mock/employee", nil)
		badRR := httptest.NewRecorder()
		mux.ServeHTTP(badRR, badReq)
		if badRR.Code != http.StatusBadRequest {
			t.Errorf("ожидался статус 400, получен %d", badRR.Code)
		}
	})

	t.Run("GET /api/mock/employee/{id} (успех и ошибка валидации)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/mock/employee/emp-777", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("ожидался статус 200, получен %d", rr.Code)
		}
		var resp EmployeeResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil || resp.EmployeeID != "emp-777" {
			t.Errorf("ожидался employee_id 'emp-777', получен '%s' (err: %v)", resp.EmployeeID, err)
		}

		badReq := httptest.NewRequest(http.MethodGet, "/api/mock/employee/%20", nil)
		badRR := httptest.NewRecorder()
		mux.ServeHTTP(badRR, badReq)
		if badRR.Code != http.StatusBadRequest {
			t.Errorf("ожидался статус 400 при пустом id, получен %d", badRR.Code)
		}
	})
}