package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const presentationSchema = `CREATE TABLE IF NOT EXISTS presentation_artifacts (
 id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES demo_chat_sessions(id),
 pdf BYTEA NOT NULL, expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '1 hour'
);`

// Anchored commands deliberately reject negations, questions and arbitrary identities.
var explicitPresentation = regexp.MustCompile(`(?i)^(?:пожалуйста[, ]+)?(?:создай|сделай|сформируй|подготовь|собери|сгенерируй)\s+(?:мне\s+)?(?:pdf[- ]?|пдф[- ]?)?(?:демо-)?(?:черновик презентации|презентацию|слайды)(?:\s+по (?:(?:моим )?(?:(?:выполненным|завершённым|завершенным) )?задачам|итогам (?:моей )?работы))?(?:[, ]+пожалуйста)?[.!]?$`)

type PresentationAttachment struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}
type presentationTask struct {
	Key         string `json:"key"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}
type presentationBullet struct {
	Text       string   `json:"text"`
	SourceKeys []string `json:"source_keys"`
}
type presentationSlide struct {
	Kind       string               `json:"kind"`
	Title      string               `json:"title"`
	Bullets    []presentationBullet `json:"bullets"`
	IsProposal bool                 `json:"is_proposal,omitempty"`
}
type presentationDraft struct {
	Slides []presentationSlide `json:"slides"`
}
type presentationGenerator func(context.Context, []presentationTask) (presentationDraft, error)

type presentationError struct {
	Status int
	Code   string
}

func (e *presentationError) Error() string { return e.Code }

var presentationTitles = []string{"Мои результаты", "Краткие итоги", "Выполненные задачи", "Чему научился", "Возможные следующие шаги"}

const nextStepProposal = "Обсудить следующие задачи с наставником."

func validatePresentationTasks(tasks []presentationTask) error {
	if len(tasks) < 1 || len(tasks) > 50 {
		return &presentationError{422, "invalid_presentation_input"}
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		if !validPresentationText(task.Key, 128) || !validPresentationText(task.Summary, 500) || (task.Description != "" && !validPresentationText(task.Description, 8000)) || seen[task.Key] {
			return &presentationError{422, "invalid_presentation_input"}
		}
		seen[task.Key] = true
	}
	return nil
}

func setDemoCookie(w http.ResponseWriter, r *http.Request, session string) {
	// Remove the former narrower cookie to avoid two identities with the same name.
	http.SetCookie(w, &http.Cookie{Name: "demo_chat", Path: "/api/chat", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
	http.SetCookie(w, &http.Cookie{Name: "demo_chat", Value: session, Path: "/api", MaxAge: 86400, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
}

func presentationClient(client *http.Client, baseURL, token string) presentationGenerator {
	return func(ctx context.Context, tasks []presentationTask) (presentationDraft, error) {
		if err := validatePresentationTasks(tasks); err != nil {
			return presentationDraft{}, err
		}
		endpoint, err := url.Parse(baseURL)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || len(token) < 32 {
			return presentationDraft{}, &presentationError{503, "ai_unavailable"}
		}
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/presentation/draft"
		body, err := json.Marshal(struct {
			Tasks []presentationTask `json:"tasks"`
		}{tasks})
		if err != nil {
			return presentationDraft{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
		if err != nil {
			return presentationDraft{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-AI-Internal-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			var timeout interface{ Timeout() bool }
			if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
				return presentationDraft{}, &presentationError{504, "ai_timeout"}
			}
			return presentationDraft{}, &presentationError{503, "ai_unavailable"}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			switch resp.StatusCode {
			case 422:
				return presentationDraft{}, &presentationError{422, "invalid_presentation_input"}
			case 503:
				return presentationDraft{}, &presentationError{503, "ai_unavailable"}
			case 504:
				return presentationDraft{}, &presentationError{504, "ai_timeout"}
			case 502:
				var failure ErrorResponse
				if json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&failure) == nil && failure.Code == "incomplete_presentation" {
					return presentationDraft{}, &presentationError{502, "incomplete_presentation"}
				}
				return presentationDraft{}, &presentationError{502, "ai_invalid_response"}
			default:
				return presentationDraft{}, &presentationError{502, "ai_invalid_response"}
			}
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		if err != nil || len(raw) > 1<<20 {
			return presentationDraft{}, &presentationError{502, "ai_invalid_response"}
		}
		var draft presentationDraft
		if err = decodeAgentArguments(raw, &draft); err != nil {
			return presentationDraft{}, &presentationError{502, "ai_invalid_response"}
		}
		if err = validatePresentation(draft, tasks); err != nil {
			return presentationDraft{}, presentationValidationError(err)
		}
		return draft, nil
	}
}

func validPresentationText(s string, limit int) bool {
	if strings.TrimSpace(s) == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > limit {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

func normalizePresentationText(s string) string { return strings.Join(strings.Fields(s), " ") }

// Cropping even a whole sentence can discard context. The agreed conservative
// contract permits only full source fields (or KEY — SUMMARY).
func presentationExtract(text string, task presentationTask) bool {
	text = normalizePresentationText(text)
	if text == normalizePresentationText(task.Key+" — "+task.Summary) {
		return true
	}
	for _, field := range []string{task.Summary, task.Description} {
		field = normalizePresentationText(field)
		if text == field {
			return true
		}
	}
	return false
}

func validatePresentation(d presentationDraft, tasks []presentationTask) error {
	if err := validatePresentationTasks(tasks); err != nil {
		return err
	}
	kinds := []string{"title", "summary", "tasks", "learning", "next_steps"}
	if len(d.Slides) != len(kinds) || len(tasks) == 0 {
		return errors.New("presentation must contain five slides and source tasks")
	}
	allowed := map[string]presentationTask{}
	for _, t := range tasks {
		allowed[t.Key] = t
	}
	covered := map[string]bool{}
	for i, s := range d.Slides {
		if s.Kind != kinds[i] || !validPresentationText(s.Title, 200) || s.Title != presentationTitles[i] || s.Bullets == nil || len(s.Bullets) > 10 || s.IsProposal != (i == 4) {
			return errors.New("invalid slide order, title, proposal flag or bullets")
		}
		if (i == 1 || i == 2) && len(s.Bullets) == 0 {
			return errors.New("empty factual slide")
		}
		if (i == 0 && len(s.Bullets) != 0) || (i == 4 && len(s.Bullets) > 1) {
			return errors.New("invalid title or next-step bullet count")
		}
		for _, b := range s.Bullets {
			if !validPresentationText(b.Text, 1000) || len(b.SourceKeys) > len(tasks) {
				return errors.New("invalid presentation bullet")
			}
			if i != 4 && len(b.SourceKeys) == 0 {
				return errors.New("factual bullet missing source keys")
			}
			seen := map[string]bool{}
			for _, key := range b.SourceKeys {
				task, ok := allowed[key]
				if !ok || seen[key] {
					return errors.New("unknown or duplicate presentation source")
				}
				seen[key] = true
				if !presentationExtract(b.Text, task) {
					return errors.New("presentation bullet is not supported by every cited task")
				}
				if i == 2 {
					covered[key] = true
				}
			}
			if i == 4 {
				if b.Text != nextStepProposal || len(b.SourceKeys) != 0 {
					return errors.New("unsupported next-step proposal")
				}
			}
		}
	}
	if len(covered) != len(allowed) {
		return &presentationError{502, "incomplete_presentation"}
	}
	return nil
}

func presentationValidationError(err error) error {
	var known *presentationError
	if errors.As(err, &known) {
		return known
	}
	return &presentationError{502, "ai_invalid_response"}
}

func createPresentation(ctx context.Context, tx *sql.Tx, session, userID string, generate presentationGenerator) (ChatResponse, error) {
	issues, err := completedDemoJiraIssues(userID)
	if err != nil {
		return ChatResponse{}, err
	}
	if len(issues) == 0 {
		return ChatResponse{Reply: "Нет завершённых демо-задач для презентации. PDF не создан."}, nil
	}
	tasks := make([]presentationTask, 0, len(issues))
	for _, issue := range issues {
		tasks = append(tasks, presentationTask{issue.Key, issue.Summary, issue.Description})
	}
	draft, err := generate(ctx, tasks)
	if err != nil {
		return ChatResponse{}, err
	}
	if err = validatePresentation(draft, tasks); err != nil {
		return ChatResponse{}, presentationValidationError(err)
	}
	pdf, err := renderPresentation(draft, tasks)
	if err != nil {
		return ChatResponse{}, err
	}
	id := randomSession()
	var expires time.Time
	if err = tx.QueryRowContext(ctx, "INSERT INTO presentation_artifacts(id,session_id,pdf) VALUES($1,$2,$3) RETURNING expires_at", id, session, pdf).Scan(&expires); err != nil {
		return ChatResponse{}, &agentStorageError{err}
	}
	return ChatResponse{Reply: fmt.Sprintf("Готов демо-черновик презентации по %d завершённым задачам.", len(tasks)), Attachment: &PresentationAttachment{
		ID: id, Filename: "demo-intern-results.pdf", ContentType: "application/pdf", DownloadURL: "/api/presentations/" + id, ExpiresAt: expires.UTC(),
	}}, nil
}

func presentationDownload(store PostgresStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		cookie, err := r.Cookie("demo_chat")
		id := r.PathValue("id")
		if err != nil || len(cookie.Value) != 64 || len(id) != 64 {
			apiError(w, 404, "presentation_not_found", "Файл недоступен")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var pdf []byte
		err = store.DB.QueryRowContext(ctx, `SELECT a.pdf FROM presentation_artifacts a
   JOIN demo_chat_sessions s ON s.id=a.session_id
   WHERE a.id=$1 AND a.session_id=$2 AND a.expires_at>now() AND s.expires_at>now()`, id, cookie.Value).Scan(&pdf)
		if errors.Is(err, sql.ErrNoRows) {
			apiError(w, 404, "presentation_not_found", "Файл недоступен")
			return
		}
		if err != nil {
			apiError(w, 503, "storage_unavailable", "Не удалось получить файл")
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="demo-intern-results.pdf"`)
		w.Header().Set("Content-Length", fmt.Sprint(len(pdf)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdf)
	}
}
