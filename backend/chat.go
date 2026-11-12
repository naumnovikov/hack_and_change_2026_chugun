package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

type ChatRequest struct {
	Message string `json:"message"`
}
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}
type ChatResponse struct {
	Reply   string   `json:"reply"`
	Sources []Source `json:"sources,omitempty"`
}
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func apiError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: message, Code: code})
}
func decodeBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}
func chatHandler(client *http.Client, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input ChatRequest
		if err := decodeBody(w, r, &input); err != nil {
			apiError(w, 400, "invalid_request", "Ожидается JSON с полем message")
			return
		}
		input.Message = strings.TrimSpace(input.Message)
		if n := utf8.RuneCountInString(input.Message); n == 0 || n > 8000 {
			apiError(w, 400, "invalid_request", "message должен содержать от 1 до 8000 символов")
			return
		}
		endpoint, err := url.Parse(baseURL)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			apiError(w, 503, "ai_unavailable", "AI-сервис не настроен")
			return
		}
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/chat"
		body, _ := json.Marshal(input)
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint.String(), bytes.NewReader(body))
		if err != nil {
			apiError(w, 503, "ai_unavailable", "AI-сервис недоступен")
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			var timeout interface{ Timeout() bool }
			if errors.As(err, &timeout) && timeout.Timeout() {
				apiError(w, 504, "ai_timeout", "AI-сервис не ответил вовремя")
			} else {
				apiError(w, 503, "ai_unavailable", "AI-сервис недоступен")
			}
			return
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			apiError(w, 502, "ai_error", "AI-сервис вернул ошибку")
			return
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		var output ChatResponse
		if err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &output) != nil || strings.TrimSpace(output.Reply) == "" {
			apiError(w, 502, "ai_invalid_response", "Некорректный ответ AI-сервиса")
			return
		}
		for _, source := range output.Sources {
			link, err := url.Parse(source.URL)
			if strings.TrimSpace(source.Title) == "" || err != nil || link.Host == "" || (link.Scheme != "http" && link.Scheme != "https") {
				apiError(w, 502, "ai_invalid_response", "Некорректный источник AI-сервиса")
				return
			}
		}
		writeJSON(w, 200, output)
	}
}
