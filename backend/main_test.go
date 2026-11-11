package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
		body := bytes.NewBufferString(`{"service_name":"Jira"}`)
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

		// Пустой payload -> 400
		badReq := httptest.NewRequest(http.MethodPost, "/api/mock/access-request", bytes.NewBufferString(`{}`))
		badRR := httptest.NewRecorder()
		mux.ServeHTTP(badRR, badReq)
		if badRR.Code != http.StatusBadRequest {
			t.Errorf("ожидался статус 400, получен %d", badRR.Code)
		}
	})

	t.Run("POST /api/mock/message (успех и ошибка валидации)", func(t *testing.T) {
		body := bytes.NewBufferString(`{"recipient_id":"emp-001","text":"Привет, кто на кнопке?"}`)
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

		// Ошибка валидации -> 400
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

		// Без параметра service_name -> 400
		badReq := httptest.NewRequest(http.MethodGet, "/api/mock/employee", nil)
		badRR := httptest.NewRecorder()
		mux.ServeHTTP(badRR, badReq)
		if badRR.Code != http.StatusBadRequest {
			t.Errorf("ожидался статус 400, получен %d", badRR.Code)
		}
	})

	t.Run("GET /api/mock/employee/{id}", func(t *testing.T) {
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
	})
}