package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAgentChatIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	store, err := openStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()

	const token = "agent-chat-test-token-with-at-least-32-characters"
	var accessTicket, messageID string
	var calls atomic.Int32
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/step" || r.Header.Get("X-AI-Internal-Token") != token {
			t.Errorf("unexpected internal AI request: path=%s", r.URL.Path)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var input agentStepRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		calls.Add(1)
		switch {
		case input.Message == "Создай мне заявку на доступ к Jira" && input.ToolResult == nil:
			writeJSON(w, http.StatusOK, agentStepResponse{
				Kind: "tool_call", State: "proposed", Reply: "proposal",
				ToolCall: &proposedAgentTool{CallID: "access-call-1", Name: "create_access_request", Arguments: json.RawMessage(`{"service_name":"Jira"}`)},
			})
		case input.ToolResult != nil && input.ToolResult.Name == "create_access_request":
			if input.ToolResult.Status != "success" || input.ToolResult.Result["status"] != "created" {
				t.Errorf("unexpected access tool result: %+v", input.ToolResult)
			}
			accessTicket, _ = input.ToolResult.Result["ticket_id"].(string)
			writeJSON(w, http.StatusOK, agentStepResponse{Kind: "answer", State: "answer", Reply: "Access request saved", Sources: []Source{}})
		case input.Message == "Who owns billing?" && input.ToolResult == nil:
			if len(input.History) != 2 || input.History[0].Role != "user" ||
				input.History[0].Content != "Создай мне заявку на доступ к Jira" ||
				input.History[1].Role != "assistant" {
				t.Errorf("missing previous conversation turn: %+v", input.History)
			}
			writeJSON(w, http.StatusOK, agentStepResponse{
				Kind: "tool_call", State: "proposed", Reply: "proposal",
				ToolCall: &proposedAgentTool{CallID: "owner-call-1", Name: "find_employee", Arguments: json.RawMessage(`{"service_name":"billing"}`)},
			})
		case input.ToolResult != nil && input.ToolResult.Name == "find_employee":
			if input.ToolResult.Result["employee_id"] != "demo-owner-billing" {
				t.Errorf("unexpected employee result: %+v", input.ToolResult)
			}
			writeJSON(w, http.StatusOK, agentStepResponse{Kind: "answer", State: "draft", Reply: "owner found", Sources: []Source{}})
		case input.Message == "подтверждаю" && input.ConfirmedMessage != nil && input.ToolResult == nil:
			writeJSON(w, http.StatusOK, agentStepResponse{
				Kind: "tool_call", State: "proposed", Reply: "proposal",
				ToolCall: &proposedAgentTool{CallID: "message-call-1", Name: "send_message", Arguments: mustJSONRaw(t, *input.ConfirmedMessage)},
			})
		case input.ToolResult != nil && input.ToolResult.Name == "send_message":
			messageID, _ = input.ToolResult.Result["message_id"].(string)
			writeJSON(w, http.StatusOK, agentStepResponse{Kind: "answer", State: "answer", Reply: "Message saved", Sources: []Source{}})
		default:
			t.Errorf("unexpected agent input: %+v", input)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer ai.Close()

	server := httptest.NewServer(agentChat(store, agentStepClient(ai.Client(), ai.URL, token), token))
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := server.Client()
	client.Jar = jar
	send := func(message, requestID string) (int, ChatResponse) {
		t.Helper()
		body, _ := json.Marshal(ChatRequest{Message: message})
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/chat", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Request-ID", requestID)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var output ChatResponse
		if err = json.NewDecoder(resp.Body).Decode(&output); err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, output
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := store.DB.QueryRowContext(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	beforeAccess, beforeMessages := count("mock_access_requests"), count("mock_messages")
	status, access := send("Создай мне заявку на доступ к Jira", "agent-access-001")
	if status != http.StatusOK || !strings.Contains(access.Reply, "DEMO-ACCESS-") {
		t.Fatalf("access result: status=%d response=%+v", status, access)
	}
	defer func() {
		if accessTicket != "" {
			_, _ = store.DB.ExecContext(context.Background(), "DELETE FROM mock_access_requests WHERE id=$1", strings.TrimPrefix(accessTicket, "DEMO-ACCESS-"))
		}
	}()
	status, repeated := send("Создай мне заявку на доступ к Jira", "agent-access-001")
	if status != http.StatusOK || repeated.Reply != access.Reply || count("mock_access_requests") != beforeAccess+1 {
		t.Fatalf("replayed access request was not idempotent: status=%d response=%+v", status, repeated)
	}

	status, draft := send("Who owns billing?", "agent-owner-001")
	if status != http.StatusOK || !strings.Contains(draft.Reply, "demo-owner-billing") || !strings.Contains(draft.Reply, "подтверждаю") {
		t.Fatalf("owner draft: status=%d response=%+v", status, draft)
	}
	status, sent := send("подтверждаю", "agent-confirm-001")
	if status != http.StatusOK || !strings.Contains(sent.Reply, "DEMO-MSG-") {
		t.Fatalf("confirmed message: status=%d response=%+v", status, sent)
	}
	defer func() {
		if messageID != "" {
			_, _ = store.DB.ExecContext(context.Background(), "DELETE FROM mock_messages WHERE id=$1", strings.TrimPrefix(messageID, "DEMO-MSG-"))
		}
	}()
	if count("mock_messages") != beforeMessages+1 {
		t.Fatal("confirmation did not create exactly one demo message")
	}
	if calls.Load() != 6 {
		t.Fatalf("AI step count = %d, want 6", calls.Load())
	}
}

func mustJSONRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAgentExplicitAccessRequiresIntent(t *testing.T) {
	for _, input := range []string{
		"Создай мне заявку на доступ к Jira",
		"Оформи доступ к Confluence",
	} {
		if !explicitAgentAccess.MatchString(input) {
			t.Errorf("explicit request did not match: %q", input)
		}
	}
	for _, input := range []string{
		"Как оформить доступ к Jira?",
		"Не создавай заявку на доступ к Jira",
		"Нужен доступ к Jira",
	} {
		if explicitAgentAccess.MatchString(input) {
			t.Errorf("non-explicit question matched: %q", input)
		}
	}
}

func TestDecodeAgentArgumentsRejectsExtraFields(t *testing.T) {
	var args demoServiceArgs
	err := decodeAgentArguments(json.RawMessage(`{"service_name":"Jira","user_id":"attacker"}`), &args)
	if err == nil {
		t.Fatal("unexpected fields must be rejected")
	}
}

func TestAgentChatRequiresLongToken(t *testing.T) {
	handler := agentChat(PostgresStore{}, nil, "short")
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("X-Request-ID", fmt.Sprintf("%08d", 1))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestConversationHistoryOnlyIncludesAvitoPlaybookSources(t *testing.T) {
	tests := []struct {
		name    string
		sources []Source
		want    bool
	}{
		{name: "source-less clarification", want: true},
		{name: "Avito Playbook", sources: []Source{{Title: "SPT", URL: "https://github.com/avito-tech/playbook/blob/main/spt.md"}}, want: true},
		{name: "external source", sources: []Source{{Title: "Other", URL: "https://example.com/doc"}}, want: false},
		{name: "workspace source", sources: []Source{{Title: "Case", URL: "https://github.com/naumnovikov/hack_and_change_2026_chugun/blob/main/docs/FAQ.md"}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validConversationSources(tt.sources); got != tt.want {
				t.Fatalf("validConversationSources() = %v, want %v", got, tt.want)
			}
		})
	}
}
