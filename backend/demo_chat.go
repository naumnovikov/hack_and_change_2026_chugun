package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const demoSchema = `CREATE TABLE IF NOT EXISTS demo_chat_sessions (
 id TEXT PRIMARY KEY, state JSONB NOT NULL DEFAULT '{}', expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '24 hours'
);
CREATE TABLE IF NOT EXISTS demo_chat_results (
 session_id TEXT NOT NULL REFERENCES demo_chat_sessions(id), request_id TEXT NOT NULL,
 message TEXT NOT NULL, response JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(session_id,request_id)
);
ALTER TABLE demo_chat_results ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();`

type demoState struct {
	Kind      string `json:"kind,omitempty"`
	Service   string `json:"service,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Text      string `json:"text,omitempty"`
}

var accessCommand = regexp.MustCompile(`(?i)^(?:оформи|создай) (?:демо-заявку на доступ|заявку на доступ|доступ)(?: к (Jira|Confluence))?[.!]?$`)
var ownerCommand = regexp.MustCompile(`(?i)^найди владельца(?: сервиса)? (billing|search)[.!]?$`)
var requestIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,128}$`)

// This is a synthetic demo catalogue, never a real Avito employee registry.
var demoOwners = map[string]string{"billing": "demo-owner-billing", "search": "demo-owner-search"}

func randomSession() string {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}

// demoChat handles only explicit demo commands. All other questions go to AI.
// Confirmation and repeat results are persisted, rather than trusted from the UI.
func demoChat(store PostgresStore, fallback http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input ChatRequest
		if decodeBody(w, r, &input) != nil {
			apiError(w, 400, "invalid_request", "Ожидается JSON с полем message")
			return
		}
		input.Message = strings.TrimSpace(input.Message)
		if input.Message == "" || len([]rune(input.Message)) > 8000 {
			apiError(w, 400, "invalid_request", "message должен содержать от 1 до 8000 символов")
			return
		}
		id := r.Header.Get("X-Request-ID")
		if !requestIDPattern.MatchString(id) {
			apiError(w, 400, "invalid_request", "Передайте X-Request-ID: уникальный ключ запроса длиной 8–128 символов")
			return
		}
		session := ""
		if cookie, err := r.Cookie("demo_chat"); err == nil && len(cookie.Value) == 64 {
			if _, err = hex.DecodeString(cookie.Value); err == nil {
				session = cookie.Value
			}
		}
		if session == "" {
			session = randomSession()
		}
		setDemoCookie(w, r, session)
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, handled, conflict, err := store.processDemo(ctx, session, id, input.Message)
		if err != nil {
			apiError(w, 503, "storage_unavailable", "Не удалось подтвердить действие. Повторите с тем же X-Request-ID")
			return
		}
		if conflict {
			apiError(w, 409, "request_conflict", "Ключ запроса уже использован с другим сообщением")
			return
		}
		if handled {
			writeJSON(w, 200, result)
			return
		}
		body, _ := json.Marshal(input)
		r.Body = io.NopCloser(bytes.NewReader(body))
		fallback.ServeHTTP(w, r)
	}
}

func (s PostgresStore) processDemo(ctx context.Context, session, id, message string) (ChatResponse, bool, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ChatResponse{}, false, false, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO demo_chat_sessions(id) VALUES($1) ON CONFLICT DO NOTHING", session); err != nil {
		return ChatResponse{}, false, false, err
	}
	var raw []byte
	var expired bool
	if err = tx.QueryRowContext(ctx, "SELECT state, expires_at < now() FROM demo_chat_sessions WHERE id=$1 FOR UPDATE", session).Scan(&raw, &expired); err != nil {
		return ChatResponse{}, false, false, err
	}
	var previousMessage string
	var previousResponse []byte
	rows, err := tx.QueryContext(ctx, "SELECT message,response FROM demo_chat_results WHERE session_id=$1 AND request_id=$2", session, id)
	if err != nil {
		return ChatResponse{}, false, false, err
	}
	if rows.Next() {
		err = rows.Scan(&previousMessage, &previousResponse)
		rows.Close()
		if err != nil {
			return ChatResponse{}, false, false, err
		}
		if previousMessage != message {
			return ChatResponse{}, false, true, nil
		}
		var output ChatResponse
		err = json.Unmarshal(previousResponse, &output)
		return output, true, false, err
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return ChatResponse{}, false, false, err
	}
	var state demoState
	if err = json.Unmarshal(raw, &state); err != nil {
		return ChatResponse{}, false, false, err
	}
	reply := ""
	lower := strings.ToLower(message)
	if expired {
		state = demoState{}
		reply = "Демо-диалог истёк. Начните новый запрос; старое подтверждение недействительно."
	} else if lower == "отмена" || lower == "отмени" {
		state = demoState{}
		reply = "Демо-действие отменено. Новых записей не создано."
	} else if lower == "подтверждаю" {
		switch state.Kind {
		case "access":
			var ticket int64
			err = tx.QueryRowContext(ctx, "INSERT INTO mock_access_requests(service_name) VALUES($1) RETURNING id", state.Service).Scan(&ticket)
			reply = fmt.Sprintf("Создана демо-заявка DEMO-ACCESS-%d на %s. Статус: created. Это не выдача доступа.", ticket, state.Service)
		case "message":
			var msg int64
			err = tx.QueryRowContext(ctx, "INSERT INTO mock_messages(recipient_id,text) VALUES($1,$2) RETURNING id", state.Recipient, state.Text).Scan(&msg)
			reply = fmt.Sprintf("Демо-сообщение DEMO-MSG-%d сохранено для %s. Реальному коллеге сообщение не отправлялось.", msg, state.Recipient)
		default:
			reply = "Нет действия для подтверждения. Сначала создайте черновик."
		}
		if err != nil {
			return ChatResponse{}, false, false, err
		}
		state = demoState{}
	} else if match := accessCommand.FindStringSubmatch(message); match != nil {
		state = demoState{Kind: "access-service"}
		if match[1] != "" {
			state = accessDraft(match[1])
			reply = accessPrompt(state.Service)
		} else {
			reply = "К какому демо-сервису нужен доступ? Jira или Confluence. Для отмены напишите «отмена»."
		}
	} else if state.Kind == "access-service" && (lower == "jira" || lower == "confluence") {
		state = accessDraft(message)
		reply = accessPrompt(state.Service)
	} else if match := ownerCommand.FindStringSubmatch(message); match != nil {
		service := strings.ToLower(match[1])
		owner := demoOwners[service]
		state = demoState{Kind: "message", Service: service, Recipient: owner, Text: fmt.Sprintf("Привет! Посмотри, пожалуйста, ревью по демо-сервису %s.", service)}
		reply = fmt.Sprintf("Демо-владелец %s: %s (синтетические данные). Черновик сообщения: «%s». Для сохранения напишите «подтверждаю», для отмены — «отмена».", service, owner, state.Text)
	} else {
		// An unrelated message invalidates the previous draft. It cannot silently
		// become a service name, nor can a later confirmation execute a stale action.
		if state.Kind != "" {
			state = demoState{}
			reply = "Предыдущий черновик отменён. Действие не выполнено. Повторите команду с сервисом."
		} else {
			return ChatResponse{}, false, false, nil
		}
	}
	output := ChatResponse{Reply: reply}
	encodedState, _ := json.Marshal(state)
	encodedOutput, _ := json.Marshal(output)
	if _, err = tx.ExecContext(ctx, "UPDATE demo_chat_sessions SET state=$2 WHERE id=$1", session, string(encodedState)); err != nil {
		return ChatResponse{}, false, false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO demo_chat_results(session_id,request_id,message,response) VALUES($1,$2,$3,$4)", session, id, message, string(encodedOutput)); err != nil {
		return ChatResponse{}, false, false, err
	}
	err = tx.Commit()
	return output, true, false, err
}
func accessDraft(service string) demoState {
	if strings.EqualFold(service, "jira") {
		service = "Jira"
	} else {
		service = "Confluence"
	}
	return demoState{Kind: "access", Service: service}
}
func accessPrompt(service string) string {
	return fmt.Sprintf("Создать демо-заявку на доступ к %s? Напишите «подтверждаю» или «отмена».", service)
}
