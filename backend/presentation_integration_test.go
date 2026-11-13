package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPresentationIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	store, err := openStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	const token = "presentation-integration-token-long-enough"
	var calls atomic.Int32
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-AI-Internal-Token") != token {
			t.Error("missing internal credential")
			w.WriteHeader(403)
			return
		}
		if r.URL.Path == "/agent/step" {
			var input agentStepRequest
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.UserContext.UserID != "demo-intern-001" {
				t.Error("invalid trusted demo context")
				w.WriteHeader(400)
				return
			}
			writeJSON(w, 200, agentStepResponse{Kind: "tool_call", ToolCall: &proposedAgentTool{CallID: "presentation-call", Name: "create_presentation", Arguments: json.RawMessage(`{}`)}})
			return
		}
		var input struct {
			Tasks []presentationTask `json:"tasks"`
		}
		if r.URL.Path != "/presentation/draft" || r.Header.Get("X-AI-Internal-Token") != token || json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("invalid internal request")
			w.WriteHeader(400)
			return
		}
		tasks := input.Tasks
		calls.Add(1)
		if len(tasks) != 3 || tasks[0].Key != "DEMO-101" || tasks[2].Key != "DEMO-103" {
			t.Error("untrusted task selection")
		}
		writeJSON(w, 200, testPresentation(tasks))
	}))
	defer ai.Close()
	generate := presentationClient(ai.Client(), ai.URL, token)
	step := agentStepClient(ai.Client(), ai.URL, token)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", agentChat(store, step, token, generate))
	mux.HandleFunc("GET /api/presentations/{id}", presentationDownload(store))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := srv.Client()
	client.Jar = jar
	send := func(c *http.Client, message, key string) (int, ChatResponse) {
		t.Helper()
		body, _ := json.Marshal(ChatRequest{Message: message})
		req, _ := http.NewRequest("POST", srv.URL+"/api/chat", bytes.NewReader(body))
		req.Header.Set("X-Request-ID", key)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out ChatResponse
		if json.NewDecoder(resp.Body).Decode(&out) != nil {
			t.Fatal("invalid chat response")
		}
		return resp.StatusCode, out
	}
	status, first := send(client, "Создай презентацию", "presentation-001")
	if status != 200 || first.Attachment == nil {
		t.Fatalf("creation: %d %+v", status, first)
	}
	artifact := first.Attachment
	var session string
	if err = store.DB.QueryRow("SELECT session_id FROM presentation_artifacts WHERE id=$1", artifact.ID).Scan(&session); err != nil {
		t.Fatal(err)
	}
	defer func() {
		store.DB.Exec("DELETE FROM presentation_artifacts WHERE session_id=$1", session)
		store.DB.Exec("DELETE FROM demo_chat_results WHERE session_id=$1", session)
		store.DB.Exec("DELETE FROM demo_chat_sessions WHERE id=$1", session)
	}()
	status, replay := send(client, "Создай презентацию", "presentation-001")
	if status != 200 || replay.Attachment == nil || replay.Attachment.ID != artifact.ID || calls.Load() != 1 {
		t.Fatal("replay regenerated artifact")
	}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, r := send(client, "Создай презентацию", "presentation-001")
			if code != 200 || r.Attachment == nil || r.Attachment.ID != artifact.ID {
				t.Error("parallel replay failed")
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("parallel duplicate")
	}
	status, _ = send(client, "Сделай презентацию", "presentation-001")
	if status != 409 {
		t.Fatal("key conflict not rejected")
	}
	status, _ = send(client, "Как создать презентацию?", "presentation-question")
	if status != 502 || calls.Load() != 1 {
		t.Fatal("implicit presentation executed")
	}
	get := func(c *http.Client, path string, want int) []byte {
		t.Helper()
		resp, err := c.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("download: got %d want %d", resp.StatusCode, want)
		}
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("download cached")
		}
		if want == 200 && (resp.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(raw, []byte("%PDF-"))) {
			t.Fatal("invalid PDF response")
		}
		return raw
	}
	get(client, artifact.DownloadURL, 200)
	otherJar, _ := cookiejar.New(nil)
	other := &http.Client{Jar: otherJar}
	get(other, artifact.DownloadURL, 404)
	send(other, "Как создать презентацию?", "other-session-001")
	get(other, artifact.DownloadURL, 404)
	get(client, "/api/presentations/"+strings.Repeat("0", 64), 404)
	// The artifact remains available through a new handler instance (no in-memory state).
	mux2 := http.NewServeMux()
	mux2.HandleFunc("GET /api/presentations/{id}", presentationDownload(store))
	req := httptest.NewRequest("GET", artifact.DownloadURL, nil)
	req.AddCookie(&http.Cookie{Name: "demo_chat", Value: session})
	rr := httptest.NewRecorder()
	mux2.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatal("restart lost artifact")
	}
	store.DB.Exec("UPDATE presentation_artifacts SET expires_at=now()-interval '1 second' WHERE id=$1", artifact.ID)
	get(client, artifact.DownloadURL, 404)
	status, replay = send(client, "Создай презентацию", "presentation-001")
	if status != 200 || replay.Attachment.ID != artifact.ID || calls.Load() != 1 {
		t.Fatal("expiry regenerated artifact")
	}
	store.DB.Exec("UPDATE presentation_artifacts SET expires_at=now()+interval '1 hour' WHERE id=$1", artifact.ID)
	store.DB.Exec("UPDATE demo_chat_sessions SET expires_at=now()-interval '1 second' WHERE id=$1", session)
	get(client, artifact.DownloadURL, 404)
}

func TestPresentationRollbackAndConcurrentCreation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	store, err := openStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	session := randomSession()
	defer func() {
		store.DB.Exec("DELETE FROM presentation_artifacts WHERE session_id=$1", session)
		store.DB.Exec("DELETE FROM demo_chat_results WHERE session_id=$1", session)
		store.DB.Exec("DELETE FROM demo_chat_sessions WHERE id=$1", session)
	}()
	ctx := context.Background()
	step := func(context.Context, agentStepRequest) (agentStepResponse, error) {
		t.Error("explicit presentation must bypass the legacy agent endpoint")
		return agentStepResponse{Kind: "tool_call", ToolCall: &proposedAgentTool{CallID: "p", Name: "create_presentation", Arguments: json.RawMessage(`{}`)}}, nil
	}
	fail := func(context.Context, []presentationTask) (presentationDraft, error) {
		return presentationDraft{}, &presentationError{504, "ai_timeout"}
	}
	if _, _, err = store.processAgent(ctx, session, "concurrent-001", "Создай презентацию", step, fail); err == nil {
		t.Fatal("failure accepted")
	}
	var n int
	if err = store.DB.QueryRow("SELECT count(*) FROM presentation_artifacts WHERE session_id=$1", session).Scan(&n); err != nil || n != 0 {
		t.Fatal("failure left artifact")
	}
	if err = store.DB.QueryRow("SELECT count(*) FROM demo_chat_results WHERE session_id=$1", session).Scan(&n); err != nil || n != 0 {
		t.Fatal("failure left success response")
	}
	var calls atomic.Int32
	generate := func(ctx context.Context, tasks []presentationTask) (presentationDraft, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return testPresentation(tasks), nil
	}
	ids := make(chan string, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, conflict, err := store.processAgent(ctx, session, "concurrent-001", "Создай презентацию", step, generate)
			if err != nil || conflict || out.Attachment == nil {
				t.Error("concurrent retry failed", err)
				return
			}
			ids <- out.Attachment.ID
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Error("different artifacts for one request")
		}
	}
	if calls.Load() != 1 || first == "" {
		t.Fatal("concurrent retry generated more than once")
	}
	if err = store.DB.QueryRow("SELECT count(*) FROM presentation_artifacts WHERE session_id=$1", session).Scan(&n); err != nil || n != 1 {
		t.Fatal("concurrent retry artifact count")
	}
}

func TestPresentationEmptyAndFailures(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	store, err := openStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	empty, err := createPresentation(ctx, tx, "unused", "unknown-user", func(context.Context, []presentationTask) (presentationDraft, error) {
		t.Fatal("AI called for empty tasks")
		return presentationDraft{}, nil
	})
	if err != nil || empty.Attachment != nil {
		t.Fatal("empty selection created artifact")
	}
	if _, err = createPresentation(ctx, tx, "unused", "", nil); err == nil {
		t.Fatal("empty identity accepted")
	}
	for _, g := range []presentationGenerator{
		func(context.Context, []presentationTask) (presentationDraft, error) {
			return presentationDraft{}, errors.New("AI down")
		},
		func(context.Context, []presentationTask) (presentationDraft, error) {
			return presentationDraft{}, context.DeadlineExceeded
		},
		func(context.Context, []presentationTask) (presentationDraft, error) { return presentationDraft{}, nil },
	} {
		if _, err = createPresentation(ctx, tx, "unused", "demo-intern-001", g); err == nil {
			t.Fatal("AI failure accepted")
		}
	}
}

// Optional compatibility test against the real FastAPI endpoint, with its model
// replaced by a deterministic provider. The default suite needs no external AI.
func TestPresentationAIContract(t *testing.T) {
	address := os.Getenv("TEST_PRESENTATION_AI_URL")
	if address == "" {
		t.Skip("TEST_PRESENTATION_AI_URL required")
	}
	token := os.Getenv("TEST_PRESENTATION_AI_TOKEN")
	tasks := testTasks()
	draft, err := presentationClient(&http.Client{Timeout: 5 * time.Second}, address, token)(context.Background(), tasks)
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := renderPresentation(draft, tasks)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("AI contract did not produce valid PDF", err)
	}
}
