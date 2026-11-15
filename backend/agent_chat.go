package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type agentUserContext struct {
	UserID         string `json:"user_id"`
	Role           string `json:"role"`
	TeamID         string `json:"team_id"`
	OnboardingWeek int    `json:"onboarding_week"`
}

type agentMessage struct {
	RecipientID string `json:"recipient_id"`
	Text        string `json:"text"`
}

type agentConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type agentToolResult struct {
	CallID string         `json:"call_id"`
	Name   string         `json:"name"`
	Status string         `json:"status"`
	Result map[string]any `json:"result"`
}

type agentStepRequest struct {
	RequestID             string                  `json:"request_id"`
	ConversationID        string                  `json:"conversation_id"`
	Message               string                  `json:"message"`
	History               []agentConversationTurn `json:"history,omitempty"`
	UserContext           agentUserContext        `json:"user_context"`
	ToolSteps             int                     `json:"tool_steps"`
	AllowedServices       []string                `json:"allowed_services"`
	AllowedRecipients     []string                `json:"allowed_recipients"`
	ExplicitAccessRequest bool                    `json:"explicit_access_request"`
	ConfirmedMessage      *agentMessage           `json:"confirmed_message,omitempty"`
	ToolResult            *agentToolResult        `json:"tool_result,omitempty"`
}

type proposedAgentTool struct {
	CallID    string          `json:"call_id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type agentStepResponse struct {
	Kind     string             `json:"kind"`
	State    string             `json:"state"`
	Reply    string             `json:"reply"`
	Sources  []Source           `json:"sources"`
	ToolCall *proposedAgentTool `json:"tool_call,omitempty"`
}

type demoServiceArgs struct {
	ServiceName string `json:"service_name"`
}

type demoMessageArgs struct {
	RecipientID string `json:"recipient_id"`
	Text        string `json:"text"`
}

type agentStepper func(context.Context, agentStepRequest) (agentStepResponse, error)

type agentStorageError struct{ err error }

func (e *agentStorageError) Error() string { return e.err.Error() }
func (e *agentStorageError) Unwrap() error { return e.err }

var explicitAgentAccess = regexp.MustCompile(`(?i)^(?:please\s+)?(?:создай|оформи|запроси)\s+(?:мне\s+)?(?:демо-)?(?:заявку на доступ|доступ)(?: к (Jira|Confluence))?[.!]?$`)

var demoEmployeeDirectory = map[string]EmployeeResponse{
	"billing": {EmployeeID: "demo-owner-billing", Name: "Демо-владелец Billing", Role: "Синтетический владелец", MattermostID: "demo-owner-billing"},
	"search":  {EmployeeID: "demo-owner-search", Name: "Демо-владелец Search", Role: "Синтетический владелец", MattermostID: "demo-owner-search"},
}

func agentStepClient(client *http.Client, baseURL, token string) agentStepper {
	return func(ctx context.Context, input agentStepRequest) (agentStepResponse, error) {
		endpoint, err := url.Parse(baseURL)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return agentStepResponse{}, errors.New("AI service URL is invalid")
		}
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/agent/step"
		body, err := json.Marshal(input)
		if err != nil {
			return agentStepResponse{}, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
		if err != nil {
			return agentStepResponse{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-AI-Internal-Token", token)
		response, err := client.Do(request)
		if err != nil {
			return agentStepResponse{}, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return agentStepResponse{}, fmt.Errorf("AI agent returned HTTP %d", response.StatusCode)
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		if err != nil || len(raw) > 1<<20 {
			return agentStepResponse{}, errors.New("AI agent response is too large or unreadable")
		}
		var output agentStepResponse
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&output); err != nil {
			return agentStepResponse{}, errors.New("AI agent response is invalid")
		}
		if err = decoder.Decode(new(any)); err != io.EOF {
			return agentStepResponse{}, errors.New("AI agent response has trailing data")
		}
		if output.Kind == "answer" && strings.TrimSpace(output.Reply) != "" && output.ToolCall == nil {
			if !validAgentSources(output.Sources) {
				return agentStepResponse{}, errors.New("AI agent returned invalid sources")
			}
			return output, nil
		}
		if output.Kind == "tool_call" && output.ToolCall != nil &&
			output.ToolCall.CallID != "" && len(output.ToolCall.Arguments) > 0 {
			return output, nil
		}
		return agentStepResponse{}, errors.New("AI agent response has an invalid shape")
	}
}

func validAgentSources(sources []Source) bool {
	for _, source := range sources {
		link, err := url.Parse(source.URL)
		if strings.TrimSpace(source.Title) == "" || err != nil || link.Host == "" ||
			(link.Scheme != "http" && link.Scheme != "https") {
			return false
		}
	}
	return true
}

func validConversationSources(sources []Source) bool {
	for _, source := range sources {
		link, err := url.Parse(source.URL)
		if err != nil || link.Scheme != "https" || link.Host != "github.com" ||
			!strings.HasPrefix(link.Path, "/avito-tech/playbook/") {
			return false
		}
	}
	return true
}

func agentChat(store PostgresStore, step agentStepper, token string, presentations ...presentationGenerator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(token) < 32 {
			apiError(w, http.StatusServiceUnavailable, "agent_unavailable", "AI-agent не настроен")
			return
		}
		var input ChatRequest
		if decodeBody(w, r, &input) != nil {
			apiError(w, http.StatusBadRequest, "invalid_request", "Ожидается JSON с полем message")
			return
		}
		input.Message = strings.TrimSpace(input.Message)
		if n := utf8.RuneCountInString(input.Message); n == 0 || n > 8000 {
			apiError(w, http.StatusBadRequest, "invalid_request", "message должен содержать от 1 до 8000 символов")
			return
		}
		id := r.Header.Get("X-Request-ID")
		if !requestIDPattern.MatchString(id) {
			apiError(w, http.StatusBadRequest, "invalid_request", "Передайте X-Request-ID: уникальный ключ запроса длиной 8–128 символов")
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
		ctx, cancel := context.WithTimeout(r.Context(), 3*aiRequestTimeout())
		defer cancel()
		response, conflict, err := store.processAgent(ctx, session, id, input.Message, step, presentations...)
		if err != nil {
			log.Printf("demo AI-agent request failed: %v", err)
			var storageErr *agentStorageError
			var timeout interface{ Timeout() bool }
			var presentationErr *presentationError
			switch {
			case errors.As(err, &presentationErr):
				if presentationErr.Code == "unsupported_pdf_text" {
					apiError(w, presentationErr.Status, presentationErr.Code, "PDF не создан: текст содержит символы, не поддерживаемые встроенным шрифтом. Исходные данные не изменены.")
					return
				}
				apiError(w, presentationErr.Status, presentationErr.Code, "Не удалось сформировать демо-презентацию")
			case errors.As(err, &storageErr):
				apiError(w, http.StatusServiceUnavailable, "storage_unavailable", "Не удалось сохранить результат демо-действия")
			case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()):
				apiError(w, http.StatusGatewayTimeout, "ai_timeout", "AI-agent не ответил вовремя")
			default:
				apiError(w, http.StatusBadGateway, "ai_error", "Не удалось завершить запрос к AI-agent")
			}
			return
		}
		if conflict {
			apiError(w, http.StatusConflict, "request_conflict", "Ключ запроса уже использован с другим сообщением")
			return
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func (s PostgresStore) processAgent(ctx context.Context, session, requestID, message string, step agentStepper, presentations ...presentationGenerator) (ChatResponse, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ChatResponse{}, false, &agentStorageError{err}
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO demo_chat_sessions(id) VALUES($1) ON CONFLICT DO NOTHING", session); err != nil {
		return ChatResponse{}, false, &agentStorageError{err}
	}
	var raw []byte
	var expired bool
	if err = tx.QueryRowContext(ctx, "SELECT state, expires_at < now() FROM demo_chat_sessions WHERE id=$1 FOR UPDATE", session).Scan(&raw, &expired); err != nil {
		return ChatResponse{}, false, &agentStorageError{err}
	}
	var previousMessage string
	var previousResponse []byte
	rows, err := tx.QueryContext(ctx, "SELECT message,response FROM demo_chat_results WHERE session_id=$1 AND request_id=$2", session, requestID)
	if err != nil {
		return ChatResponse{}, false, &agentStorageError{err}
	}
	if rows.Next() {
		err = rows.Scan(&previousMessage, &previousResponse)
		rows.Close()
		if err != nil {
			return ChatResponse{}, false, &agentStorageError{err}
		}
		if previousMessage != message {
			return ChatResponse{}, true, nil
		}
		var output ChatResponse
		err = json.Unmarshal(previousResponse, &output)
		return output, false, err
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return ChatResponse{}, false, &agentStorageError{err}
	}
	rows.Close()
	var state demoState
	if err = json.Unmarshal(raw, &state); err != nil {
		return ChatResponse{}, false, err
	}
	if expired {
		state = demoState{}
		return persistAgentResult(ctx, tx, session, requestID, message, state,
			ChatResponse{Reply: "Демо-диалог истёк. Начните новый запрос; старое подтверждение недействительно."})
	}
	lower := strings.ToLower(message)
	if lower == "отмена" || lower == "отмени" {
		state = demoState{}
		return persistAgentResult(ctx, tx, session, requestID, message, state,
			ChatResponse{Reply: "Демо-действие отменено. Новых записей не создано."})
	}

	var confirmed *agentMessage
	if state.Kind == "agent-message" {
		if lower == "подтверждаю" {
			confirmed = &agentMessage{RecipientID: state.Recipient, Text: state.Text}
		} else {
			state = demoState{}
		}
	}

	var history []agentConversationTurn
	if !expired {
		previousRows, queryErr := tx.QueryContext(ctx, `SELECT message,response FROM demo_chat_results
			WHERE session_id=$1 ORDER BY created_at DESC,request_id DESC LIMIT 2`, session)
		if queryErr != nil {
			return ChatResponse{}, false, &agentStorageError{queryErr}
		}
		var previousPairs [][]agentConversationTurn
		for previousRows.Next() {
			var previousTurnMessage string
			var previousTurnResponse []byte
			if err = previousRows.Scan(&previousTurnMessage, &previousTurnResponse); err != nil {
				previousRows.Close()
				return ChatResponse{}, false, &agentStorageError{err}
			}
			var previousTurn ChatResponse
			if json.Unmarshal(previousTurnResponse, &previousTurn) == nil && strings.TrimSpace(previousTurn.Reply) != "" && validConversationSources(previousTurn.Sources) {
				previousPairs = append(previousPairs, []agentConversationTurn{{Role: "user", Content: previousTurnMessage}, {Role: "assistant", Content: previousTurn.Reply}})
			}
		}
		err = previousRows.Err()
		previousRows.Close()
		if err != nil {
			return ChatResponse{}, false, &agentStorageError{err}
		}
		for i := len(previousPairs) - 1; i >= 0; i-- {
			history = append(history, previousPairs[i]...)
		}
	}

	request := agentStepRequest{
		RequestID: requestID, ConversationID: session, Message: message,
		History:               history,
		UserContext:           agentUserContext{UserID: "demo-intern-001", Role: "Intern", TeamID: "demo-team", OnboardingWeek: 1},
		AllowedServices:       []string{"Jira", "Confluence", "billing", "search"},
		AllowedRecipients:     []string{"demo-owner-billing", "demo-owner-search"},
		ExplicitAccessRequest: explicitAgentAccess.MatchString(message),
		ConfirmedMessage:      confirmed,
	}
	// MVP routing belongs to Go: the AI draft endpoint is available independently
	// of the legacy agent tool allowlist. Identity and task selection stay trusted.
	if explicitPresentation.MatchString(message) {
		if len(presentations) != 1 || presentations[0] == nil {
			return ChatResponse{}, false, &presentationError{503, "ai_unavailable"}
		}
		response, err := createPresentation(ctx, tx, session, request.UserContext.UserID, presentations[0])
		if err != nil {
			return ChatResponse{}, false, err
		}
		return persistAgentResult(ctx, tx, session, requestID, message, demoState{}, response)
	}
	output, err := step(ctx, request)
	if err != nil {
		return ChatResponse{}, false, err
	}
	if output.Kind == "answer" {
		state = demoState{}
		return persistAgentResult(ctx, tx, session, requestID, message, state,
			ChatResponse{Reply: output.Reply, Sources: output.Sources})
	}

	tool := output.ToolCall
	if tool == nil {
		return ChatResponse{}, false, errors.New("AI agent omitted tool call")
	}
	if confirmed != nil && tool.Name != "send_message" {
		return ChatResponse{}, false, errors.New("AI agent proposed a non-message action after confirmation")
	}
	result := agentToolResult{CallID: tool.CallID, Name: tool.Name, Status: "success", Result: map[string]any{}}
	switch tool.Name {
	case "create_presentation":
		var args struct{}
		if !explicitPresentation.MatchString(message) || decodeAgentArguments(tool.Arguments, &args) != nil {
			return ChatResponse{}, false, errors.New("unauthorized presentation request")
		}
		if len(presentations) != 1 {
			return ChatResponse{}, false, errors.New("presentation generator unavailable")
		}
		response, err := createPresentation(ctx, tx, session, request.UserContext.UserID, presentations[0])
		if err != nil {
			return ChatResponse{}, false, err
		}
		return persistAgentResult(ctx, tx, session, requestID, message, demoState{}, response)
	case "create_access_request":
		var args demoServiceArgs
		if err = decodeAgentArguments(tool.Arguments, &args); err != nil ||
			!request.ExplicitAccessRequest || (args.ServiceName != "Jira" && args.ServiceName != "Confluence") {
			return ChatResponse{}, false, errors.New("AI agent proposed an unauthorized access request")
		}
		var id int64
		var status string
		err = tx.QueryRowContext(ctx, "INSERT INTO mock_access_requests(service_name) VALUES($1) RETURNING id,status", args.ServiceName).Scan(&id, &status)
		if err != nil {
			return ChatResponse{}, false, &agentStorageError{err}
		}
		ticketID := fmt.Sprintf("DEMO-ACCESS-%d", id)
		result.Result = map[string]any{"ticket_id": ticketID, "status": status}
		_ = continueAgent(ctx, step, request, result, 1)
		reply := fmt.Sprintf("Создана демо-заявка %s на %s. Статус: %s. Это ещё не выдача доступа.", ticketID, args.ServiceName, status)
		state = demoState{}
		return persistAgentResult(ctx, tx, session, requestID, message, state, ChatResponse{Reply: reply})

	case "find_employee":
		var args demoServiceArgs
		if err = decodeAgentArguments(tool.Arguments, &args); err != nil {
			return ChatResponse{}, false, errors.New("AI agent proposed invalid employee search arguments")
		}
		employee, ok := demoEmployeeDirectory[strings.ToLower(args.ServiceName)]
		if !ok {
			return ChatResponse{}, false, errors.New("AI agent proposed an unknown demo service")
		}
		result.Result = map[string]any{
			"employee_id": employee.EmployeeID, "name": employee.Name,
			"role": employee.Role, "mattermost_id": employee.MattermostID,
		}
		_ = continueAgent(ctx, step, request, result, 1)
		text := fmt.Sprintf("Привет! Посмотри, пожалуйста, ревью по демо-сервису %s.", strings.ToLower(args.ServiceName))
		state = demoState{Kind: "agent-message", Service: strings.ToLower(args.ServiceName), Recipient: employee.EmployeeID, Text: text}
		reply := fmt.Sprintf("Демо-владелец %s: %s (%s; синтетические данные). Черновик сообщения: «%s». Для отправки демо-сообщения напишите «подтверждаю», для отмены — «отмена».", strings.ToLower(args.ServiceName), employee.Name, employee.EmployeeID, text)
		return persistAgentResult(ctx, tx, session, requestID, message, state, ChatResponse{Reply: reply})

	case "send_message":
		var args demoMessageArgs
		if err = decodeAgentArguments(tool.Arguments, &args); err != nil || confirmed == nil ||
			args.RecipientID != confirmed.RecipientID || args.Text != confirmed.Text ||
			(args.RecipientID != "demo-owner-billing" && args.RecipientID != "demo-owner-search") {
			return ChatResponse{}, false, errors.New("AI agent proposed an unconfirmed demo message")
		}
		var id int64
		var sentAt time.Time
		if err = tx.QueryRowContext(ctx, "INSERT INTO mock_messages(recipient_id,text) VALUES($1,$2) RETURNING id,sent_at", args.RecipientID, args.Text).Scan(&id, &sentAt); err != nil {
			return ChatResponse{}, false, &agentStorageError{err}
		}
		messageID := fmt.Sprintf("DEMO-MSG-%d", id)
		result.Result = map[string]any{"message_id": messageID, "sent_at": sentAt.UTC().Format(time.RFC3339Nano)}
		_ = continueAgent(ctx, step, request, result, 1)
		reply := fmt.Sprintf("Демо-сообщение %s сохранено для %s. Реальному коллеге сообщение не отправлялось.", messageID, args.RecipientID)
		state = demoState{}
		return persistAgentResult(ctx, tx, session, requestID, message, state, ChatResponse{Reply: reply})
	default:
		return ChatResponse{}, false, errors.New("AI agent proposed an unsupported tool")
	}
}

func continueAgent(ctx context.Context, step agentStepper, request agentStepRequest, result agentToolResult, steps int) agentStepResponse {
	request.ToolSteps = steps
	request.ToolResult = &result
	output, err := step(ctx, request)
	if err != nil {
		log.Printf("AI agent follow-up failed after persisted demo action: %v", err)
		return agentStepResponse{}
	}
	return output
}

func decodeAgentArguments(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected trailing data in tool arguments")
	}
	return nil
}

func persistAgentResult(ctx context.Context, tx *sql.Tx, session, requestID, message string, state demoState, output ChatResponse) (ChatResponse, bool, error) {
	encodedState, err := json.Marshal(state)
	if err != nil {
		return ChatResponse{}, false, err
	}
	encodedOutput, err := json.Marshal(output)
	if err != nil {
		return ChatResponse{}, false, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE demo_chat_sessions SET state=$2 WHERE id=$1", session, string(encodedState)); err != nil {
		return ChatResponse{}, false, &agentStorageError{err}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO demo_chat_results(session_id,request_id,message,response) VALUES($1,$2,$3,$4)", session, requestID, message, string(encodedOutput)); err != nil {
		return ChatResponse{}, false, &agentStorageError{err}
	}
	if err = tx.Commit(); err != nil {
		return ChatResponse{}, false, &agentStorageError{err}
	}
	return output, false, nil
}
