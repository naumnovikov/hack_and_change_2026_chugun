package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDemoChatIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	store, err := openStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input ChatRequest
		if decodeBody(w, r, &input) != nil {
			t.Error("fallback body lost")
		}
		writeJSON(w, 200, ChatResponse{Reply: "AI: " + input.Message})
	})
	server := httptest.NewServer(demoChat(store, fallback))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := server.Client()
	client.Jar = jar
	sequence := 0
	send := func(c *http.Client, text, key string, want int) string {
		t.Helper()
		req, _ := http.NewRequest("POST", server.URL+"/api/chat", strings.NewReader(text))
		req.Header.Set("X-Request-ID", key)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("status %d want %d: %s", resp.StatusCode, want, buf.String())
		}
		return buf.String()
	}
	msg := func(text string) string {
		sequence++
		return send(client, fmt.Sprintf(`{"message":%q}`, text), fmt.Sprintf("demo-test-%d", sequence), 200)
	}
	count := func(table string) int {
		var n int
		if err := store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	beforeAccess, beforeMessage := count("mock_access_requests"), count("mock_messages")
	send(client, `!`, "demo-bad-000", 400)
	send(client, `{"message":" "}`, "demo-bad-001", 400)
	send(client, `{"message":"hi"}`, "", 400)
	if !strings.Contains(msg("Что такое SPT?"), "AI:") {
		t.Fatal("question not forwarded")
	}
	msg("Не создавай заявку на доступ к Jira")
	msg("Как создать заявку на доступ к Jira?")
	if count("mock_access_requests") != beforeAccess {
		t.Fatal("question/negation wrote action")
	}
	msg("Создай заявку на доступ")
	msg("отмена")
	msg("подтверждаю")
	if count("mock_access_requests") != beforeAccess {
		t.Fatal("cancel wrote action")
	}
	msg("Оформи доступ к Jira")
	msg("Другой вопрос")
	msg("подтверждаю")
	if count("mock_access_requests") != beforeAccess {
		t.Fatal("stale draft executed")
	}
	msg("Создай заявку на доступ")
	msg("Confluence")
	// A different cookie jar cannot confirm this session's draft.
	otherJar, _ := cookiejar.New(nil)
	other := &http.Client{Transport: client.Transport}
	other.Jar = otherJar
	send(other, `{"message":"подтверждаю"}`, "other-request-001", 200)
	if count("mock_access_requests") != beforeAccess {
		t.Fatal("cross-session confirmation")
	}
	first := send(client, `{"message":"подтверждаю"}`, "confirm-access-001", 200)
	if !strings.Contains(first, "DEMO-ACCESS-") {
		t.Fatal(first)
	}
	repeated := send(client, `{"message":"подтверждаю"}`, "confirm-access-001", 200)
	if first != repeated || count("mock_access_requests") != beforeAccess+1 {
		t.Fatal("idempotency failed")
	}
	send(client, `{"message":"отмена"}`, "confirm-access-001", 409)
	msg("подтверждаю")
	if count("mock_access_requests") != beforeAccess+1 {
		t.Fatal("second confirmation wrote action")
	}
	msg("Найди владельца сервиса billing")
	msg("отмени")
	msg("подтверждаю")
	if count("mock_messages") != beforeMessage {
		t.Fatal("cancel sent message")
	}
	draft := msg("Найди владельца search")
	if !strings.Contains(draft, "demo-owner-search") {
		t.Fatal(draft)
	}
	result := send(client, `{"message":"подтверждаю"}`, "confirm-message-001", 200)
	send(client, `{"message":"подтверждаю"}`, "confirm-message-001", 200)
	if !strings.Contains(result, "DEMO-MSG-") || count("mock_messages") != beforeMessage+1 {
		t.Fatal("message repeat failed")
	}
	// Persisted results survive construction of a new HTTP handler (server restart).
	restarted := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"message":"подтверждаю"}`))
	req.Header.Set("X-Request-ID", "confirm-message-001")
	parsed, _ := http.NewRequest("GET", server.URL+"/api/chat", nil)
	for _, cookie := range jar.Cookies(parsed.URL) {
		req.AddCookie(cookie)
	}
	demoChat(store, fallback)(restarted, req)
	if restarted.Body.String() != result {
		t.Fatal("result lost across handler restart")
	}
	msg("Оформи доступ к Jira")
	_, err = store.DB.ExecContext(context.Background(), "UPDATE demo_chat_sessions SET expires_at=now()-interval '1 hour' WHERE id=$1", jar.Cookies(parsed.URL)[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg("подтверждаю"), "истёк") {
		t.Fatal("expired draft executed")
	}
}
func TestAccessDraft(t *testing.T) {
	if accessDraft("JIRA").Service != "Jira" || accessDraft("confluence").Service != "Confluence" {
		t.Fatal("catalogue normalization")
	}
}
