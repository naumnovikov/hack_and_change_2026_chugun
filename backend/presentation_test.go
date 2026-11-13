package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testPresentation(tasks []presentationTask) presentationDraft {
	d := presentationDraft{}
	for idx, kind := range []string{"title", "summary", "tasks", "learning", "next_steps"} {
		s := presentationSlide{Kind: kind, Title: presentationTitles[idx], Bullets: []presentationBullet{}, IsProposal: kind == "next_steps"}
		if kind != "title" && kind != "next_steps" {
			for _, task := range tasks {
				s.Bullets = append(s.Bullets, presentationBullet{Text: task.Key + " — " + task.Summary, SourceKeys: []string{task.Key}})
			}
		}
		d.Slides = append(d.Slides, s)
	}
	return d
}

func testTasks() []presentationTask {
	issues, _ := completedDemoJiraIssues("demo-intern-001")
	tasks := []presentationTask{}
	for _, i := range issues {
		tasks = append(tasks, presentationTask{i.Key, i.Summary, i.Description})
	}
	return tasks
}

func TestPresentationValidation(t *testing.T) {
	tasks := testTasks()
	for _, tc := range []struct {
		name   string
		mutate func(*presentationDraft)
		valid  bool
	}{
		{"valid", func(d *presentationDraft) {}, true},
		{"wrong order", func(d *presentationDraft) { d.Slides[1].Kind = "tasks" }, false},
		{"missing slide", func(d *presentationDraft) { d.Slides = d.Slides[:4] }, false},
		{"blank title", func(d *presentationDraft) { d.Slides[0].Title = " " }, false},
		{"long text", func(d *presentationDraft) { d.Slides[1].Bullets[0].Text = strings.Repeat("я", 1201) }, false},
		{"control", func(d *presentationDraft) { d.Slides[1].Bullets[0].Text = "bad\x00" }, false},
		{"unknown key", func(d *presentationDraft) { d.Slides[1].Bullets[0].SourceKeys = []string{"DEMO-105"} }, false},
		{"missing key", func(d *presentationDraft) { d.Slides[1].Bullets[0].SourceKeys = nil }, false},
		{"duplicate key", func(d *presentationDraft) { d.Slides[1].Bullets[0].SourceKeys = []string{"DEMO-101", "DEMO-101"} }, false},
		{"empty learning", func(d *presentationDraft) { d.Slides[3].Bullets = []presentationBullet{} }, true},
		{"empty summary", func(d *presentationDraft) { d.Slides[1].Bullets = []presentationBullet{} }, false},
		{"invented skill", func(d *presentationDraft) { d.Slides[3].Bullets[0].Text = "Освоил Kubernetes" }, false},
		{"bad proposal", func(d *presentationDraft) {
			d.Slides[4].Bullets = []presentationBullet{{Text: "Релиз завтра"}}
		}, false},
		{"missing task", func(d *presentationDraft) { d.Slides[2].Bullets = d.Slides[2].Bullets[:1] }, false},
		{"unmarked proposal", func(d *presentationDraft) { d.Slides[4].IsProposal = false }, false},
		{"null bullets", func(d *presentationDraft) { d.Slides[0].Bullets = nil }, false},
		{"title facts", func(d *presentationDraft) { d.Slides[0].Bullets = d.Slides[1].Bullets }, false},
		{"unrelated citation", func(d *presentationDraft) {
			d.Slides[1].Bullets[0].SourceKeys = append(d.Slides[1].Bullets[0].SourceKeys, "DEMO-102")
		}, false},
		{"duplicate proposal", func(d *presentationDraft) {
			d.Slides[4].Bullets = []presentationBullet{{Text: nextStepProposal}, {Text: nextStepProposal}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testPresentation(tasks)
			tc.mutate(&d)
			if (validatePresentation(d, tasks) == nil) != tc.valid {
				t.Fatal("unexpected validation result")
			}
		})
	}
	if validatePresentation(testPresentation(tasks), nil) == nil {
		t.Fatal("empty tasks accepted")
	}
}

func TestPresentationExtractPreservesMeaning(t *testing.T) {
	task := presentationTask{Key: "DEMO-101", Summary: "Проверка доступа", Description: "Не выполнена проверка прав. Добавлена   валидация\nназвания сервиса."}
	for _, text := range []string{"Проверка доступа", "DEMO-101 — Проверка доступа", "Не выполнена проверка прав. Добавлена валидация названия сервиса."} {
		if !presentationExtract(text, task) {
			t.Errorf("safe extract rejected: %s", text)
		}
	}
	for _, text := range []string{"Не выполнена проверка прав.", "Добавлена валидация названия сервиса.", "выполнена проверка прав.", "валидация названия", "Освоил Kubernetes"} {
		if presentationExtract(text, task) {
			t.Errorf("unsafe extract accepted: %s", text)
		}
	}
	task.Description = ""
	if err := validatePresentationTasks([]presentationTask{task}); err != nil {
		t.Fatal("empty description must be allowed")
	}
}

func TestPresentationClient(t *testing.T) {
	tasks := testTasks()
	good, _ := json.Marshal(testPresentation(tasks))
	const token = "presentation-internal-token-32-characters"
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"valid", string(good), 200, true},
		{"invalid JSON", "!", 200, false},
		{"trailing", string(good) + " {}", 200, false},
		{"unknown field", `{"slides":[],"file":"attacker"}`, 200, false},
		{"invalid slides", `{"slides":[]}`, 200, false},
		{"error", "private error", 500, false},
		{"oversize", strings.Repeat("x", (1<<20)+1), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/presentation/draft" || r.Header.Get("X-AI-Internal-Token") != token {
					t.Error("wrong AI request")
				}
				var input struct {
					Tasks []presentationTask `json:"tasks"`
				}
				if json.NewDecoder(r.Body).Decode(&input) != nil || !reflect.DeepEqual(input.Tasks, tasks) {
					t.Error("wrong task payload")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := presentationClient(srv.Client(), srv.URL, token)(context.Background(), tasks)
			if (err == nil) != tc.valid {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
	for _, base := range []string{"!", "file:///tmp", "http://"} {
		if _, err := presentationClient(http.DefaultClient, base, token)(context.Background(), tasks); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := presentationClient(http.DefaultClient, "http://localhost:1", token)(ctx, tasks); err == nil {
		t.Fatal("cancel ignored")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(20 * time.Millisecond) }))
	defer slow.Close()
	if _, err := presentationClient(&http.Client{Timeout: time.Millisecond}, slow.URL, token)(context.Background(), tasks); err == nil {
		t.Fatal("timeout ignored")
	}
}

func TestPresentationIntent(t *testing.T) {
	for _, s := range []string{"Создай презентацию", "Подготовь мне PDF-презентацию по моим завершённым задачам", "Собери черновик презентации", "Собери демо-черновик презентации по моим завершённым задачам"} {
		if !explicitPresentation.MatchString(s) {
			t.Errorf("command rejected: %s", s)
		}
	}
	for _, s := range []string{"Не создавай презентацию", "Как создать презентацию?", "Создай презентацию для demo-intern-002", "создай презентацию?"} {
		if explicitPresentation.MatchString(s) {
			t.Errorf("unsafe intent: %s", s)
		}
	}
}

func TestPresentationMissingDownloadCookie(t *testing.T) {
	rr := httptest.NewRecorder()
	presentationDownload(PostgresStore{})(rr, httptest.NewRequest("GET", "/api/presentations/nope", nil))
	if rr.Code != 404 || rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unsafe missing-cookie response")
	}
}

func TestPresentationInputLimitsAndStatusCodes(t *testing.T) {
	task := presentationTask{Key: strings.Repeat("K", 128), Summary: strings.Repeat("я", 500), Description: strings.Repeat("я", 8000)}
	if err := validatePresentationTasks([]presentationTask{task}); err != nil {
		t.Fatal("exact limits rejected")
	}
	for _, tasks := range [][]presentationTask{nil, make([]presentationTask, 51), {task, task}, {{Key: task.Key + "x", Summary: "s", Description: "d"}}, {{Key: "K", Summary: task.Summary + "я", Description: "d"}}, {{Key: "K", Summary: "s", Description: task.Description + "я"}}} {
		var pe *presentationError
		if !errors.As(validatePresentationTasks(tasks), &pe) || pe.Status != 422 {
			t.Fatal("invalid input must be 422")
		}
	}
	for _, status := range []int{422, 502, 503, 504, 401} {
		t.Run(fmt.Sprintf("HTTP%d", status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer srv.Close()
			_, err := presentationClient(srv.Client(), srv.URL, strings.Repeat("t", 32))(context.Background(), testTasks())
			var pe *presentationError
			expected := status
			if status == 401 {
				expected = 502
			}
			if !errors.As(err, &pe) || pe.Status != expected {
				t.Fatalf("wrong mapped status: %v", err)
			}
		})
	}
}

func TestIncompletePresentationIsTypedAndPreserved(t *testing.T) {
	tasks := testTasks()
	draft := testPresentation(tasks)
	draft.Slides[2].Bullets = draft.Slides[2].Bullets[:1]
	err := presentationValidationError(validatePresentation(draft, tasks))
	var pe *presentationError
	if !errors.As(err, &pe) || pe.Status != 502 || pe.Code != "incomplete_presentation" {
		t.Fatal("incomplete result lost its error code", err)
	}
	for _, upstream := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if upstream {
				writeJSON(w, 502, ErrorResponse{Code: "incomplete_presentation", Error: "private details"})
			} else {
				writeJSON(w, 200, draft)
			}
		}))
		_, err = presentationClient(srv.Client(), srv.URL, strings.Repeat("t", 32))(context.Background(), tasks)
		srv.Close()
		if !errors.As(err, &pe) || pe.Code != "incomplete_presentation" || pe.Status != 502 {
			t.Fatal("incomplete upstream result not preserved", err)
		}
	}
}

func TestLargePresentationDoesNotSilentlyDropTasks(t *testing.T) {
	tasks := make([]presentationTask, 50)
	for i := range tasks {
		tasks[i] = presentationTask{Key: fmt.Sprintf("DEMO-%d", i), Summary: fmt.Sprintf("Результат %d", i), Description: ""}
	}
	if err := validatePresentationTasks(tasks); err != nil {
		t.Fatal("50 tasks must be a valid input", err)
	}
	draft := testPresentation(tasks)
	draft.Slides[1].Bullets = draft.Slides[1].Bullets[:10]
	draft.Slides[2].Bullets = draft.Slides[2].Bullets[:10]
	draft.Slides[3].Bullets = []presentationBullet{}
	var pe *presentationError
	err := validatePresentation(draft, tasks)
	if !errors.As(err, &pe) || pe.Status != 502 || pe.Code != "incomplete_presentation" {
		t.Fatal("truncated selection accepted", err)
	}
}
