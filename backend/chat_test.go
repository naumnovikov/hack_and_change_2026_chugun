package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatProxy(t *testing.T) {
	for _, tc := range []struct {
		input, output string
		status, want  int
	}{
		{`{"message":"hi"}`, `{"reply":"hello","sources":[{"title":"Demo","url":"https://example.org"}]}`, 200, 200},
		{`{"message":"hi"}`, `{"reply":"hello"}`, 200, 200},
		{`{"message":" "}`, `{}`, 200, 400}, {`!`, `{}`, 200, 400}, {`{}`, `{}`, 200, 400},
		{`{"message":"hi","user_id":"other"}`, `{}`, 200, 400},
		{`{"message":"hi"} {}`, `{}`, 200, 400},
		{`{"message":"` + strings.Repeat("я", 8001) + `"}`, `{}`, 200, 400},
		{`{"message":"hi"}`, `private diagnostics`, 500, 502},
		{`{"message":"hi"}`, `!`, 200, 502}, {`{"message":"hi"}`, `{}`, 200, 502},
		{`{"message":"hi"}`, `{"reply":"hi","sources":[{"title":"Demo","url":"javascript:alert(1)"}]}`, 200, 502},
		{`{"message":"hi"}`, strings.Repeat("a", (1<<20)+1), 200, 502},
	} {
		t.Run(tc.input[:1]+tc.output[:1], func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.URL.Path != "/chat" || r.Method != "POST" {
					t.Error("wrong upstream contract")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.output))
			}))
			defer server.Close()
			rr := httptest.NewRecorder()
			chatHandler(server.Client(), server.URL)(rr, httptest.NewRequest("POST", "/api/chat", strings.NewReader(tc.input)))
			if rr.Code != tc.want {
				t.Fatalf("%d %s", rr.Code, rr.Body.String())
			}
			if tc.want == 400 && called {
				t.Fatal("invalid input forwarded")
			}
			if tc.want == 200 && strings.Contains(tc.output, "sources") && !strings.Contains(rr.Body.String(), "https://example.org") {
				t.Fatal("lost source")
			}
		})
	}
}
func TestChatFailures(t *testing.T) {
	for _, address := range []string{"", "::bad", "ftp://example.org", "http://127.0.0.1:1"} {
		rr := httptest.NewRecorder()
		chatHandler(&http.Client{Timeout: time.Second}, address)(rr, httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"message":"hi"}`)))
		if rr.Code != 503 {
			t.Fatal(rr.Code)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(30 * time.Millisecond) }))
	defer server.Close()
	rr := httptest.NewRecorder()
	chatHandler(&http.Client{Timeout: time.Millisecond}, server.URL)(rr, httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"message":"hi"}`)))
	if rr.Code != 504 {
		t.Fatal(rr.Code)
	}
}

type fakeStore struct {
	err    error
	writes int
}

func (s *fakeStore) CreateAccess(context.Context, string) (AccessRequestResponse, error) {
	s.writes++
	return AccessRequestResponse{TicketID: "DEMO-1", Status: "created"}, s.err
}
func (s *fakeStore) SendMessage(context.Context, string, string) (SendMessageResponse, error) {
	s.writes++
	return SendMessageResponse{MessageID: "DEMO-2", SentAt: "2026-11-12T00:00:00Z"}, s.err
}
func TestPersistentActions(t *testing.T) {
	for _, tc := range []struct {
		path, body string
		fail       bool
		want       int
	}{
		{"access-request", `{"service_name":"Jira"}`, false, 200}, {"access-request", `{"service_name":" "}`, false, 400}, {"access-request", `!`, false, 400}, {"access-request", `{"service_name":"Jira"}`, true, 503},
		{"message", `{"recipient_id":"demo-001","text":"Review DEMO-42"}`, false, 200}, {"message", `{"recipient_id":"demo-001","text":" "}`, false, 400}, {"message", `!`, false, 400}, {"message", `{"recipient_id":"demo-001","text":"Review DEMO-42"}`, true, 503},
	} {
		s := &fakeStore{}
		if tc.fail {
			s.err = errors.New("private db error")
		}
		rr := httptest.NewRecorder()
		persistentActions(s).ServeHTTP(rr, httptest.NewRequest("POST", "/api/mock/"+tc.path, strings.NewReader(tc.body)))
		if rr.Code != tc.want {
			t.Fatalf("%d %s", rr.Code, rr.Body.String())
		}
		if tc.want == 400 && s.writes != 0 {
			t.Fatal("invalid write")
		}
		if strings.Contains(rr.Body.String(), "private") {
			t.Fatal("leaked diagnostics")
		}
	}
}
