# Backend: PDF presentation integration

Scope: `specs/presentation.spec.md`. All input tasks are synthetic.

Go routes an anchored explicit Russian presentation command directly to the
internal AI draft endpoint, for example
`Создай презентацию` or `Подготовь презентацию по моим завершённым задачам`.
The user ID comes from the existing server-owned demo context, never tool arguments.
There is no new identity field or external Jira call.

Go sends only filtered `key`, `summary`, `description` to the internal
`POST /presentation/draft`, using the existing `X-AI-Internal-Token`.
Five ordered slides and source keys are validated before rendering. Structural
validation accepts full source fields or
`KEY — SUMMARY`; whitespace is normalized on both sides. Arbitrary substrings,
free paraphrases, cropped negations and skill inferences are rejected. Every
cited task must support the text, not just one task in source_keys.
Titles are fixed to the examples in presentation.spec.md. Empty learning bullets
are allowed. The only next-step bullet is `Обсудить следующие задачи с наставником.`
No unknown source keys are accepted; every selected task must
be represented on the task slide. The task slide also includes original fixture
summaries and descriptions, so facts remain inspectable independently of AI wording.

PDF bytes, the session-bound artifact and the saved chat response commit atomically
in Postgres, using the existing session lock and `X-Request-ID` replay mechanism.
The current coordinator holds that lock/transaction during AI work, as for existing
agent operations; concurrent requests in one session wait for the active request.
An AI failure rolls back, so the same request key can be retried. Expired artifact
replays return the same expired attachment rather than silently generating a new PDF.
To intentionally generate another draft, send a new explicit request with a new key.

Downloads require the owning `demo_chat` cookie and an unexpired session and artifact.
TTL is one hour, measured by Postgres. Missing, foreign and expired artifacts return
the same 404. Download responses use `Cache-Control: no-store` and `nosniff`.
Cookie scope migrates from `/api/chat` to `/api`, removing the old narrower cookie.
Expired artifacts are inaccessible but retained in this demo database; scheduled
physical cleanup and permanent storage are outside this change.

The AI endpoint is merged in PR #30. Explicit presentation commands now bypass
the legacy `/agent/step` and call `/presentation/draft` directly from Go, using
the existing trusted demo context. Frontend must accept optional `attachment`
according to Backend OpenAPI; ordinary chat responses remain compatible.
Without `AI_INTERNAL_TOKEN`, the existing fallback chat remains unchanged and
does not generate PDF files. The former people-search code is retained until its
coordinated removal; this PR adds the new flow without changing other participants' files.

Tests: `go test -coverprofile=coverage.out ./...` with `TEST_DATABASE_URL` pointing
to an isolated synthetic PostgreSQL database. Without it integration tests skip.
The integration tests cover replay, conflict, parallel replay, source filtering,
download ownership, restart, expiry, invalid content and empty selection.
The transport test exercises the draft endpoint against a fake AI server;
another test rolls back a failed generation and retries the same key concurrently
four times, asserting a single model invocation and a single committed artifact.

## Selected MVP routing

After the lead authorized choosing an integration path to unblock development,
Go directly routes anchored explicit presentation commands to `/presentation/draft`.
Other messages retain the existing agent path. No new AI request fields are needed.
The agreed policy from PR #30 fixes full
task coverage and safe extracts. Backend and AI now require all selected tasks
on the task slide and full-field grounding; AI responses which fail
these checks are rejected safely with 502, even if AI returned HTTP 200.
Missing task coverage returns `incomplete_presentation`, preserved from AI or
detected independently in Go. Input up to 50 tasks remains allowed; no batching,
selection truncation or partial PDF is introduced to fit the ten-bullet limit.

## PDF dependencies and distribution

Fixed versions: `github.com/go-pdf/fpdf v0.9.0` (MIT) and
`golang.org/x/image v0.18.0` (BSD-style license). Go Regular TTF is compiled into
the executable through the goregular package; no OS font directory is consulted.
The font is Copyright (c) 2016 Bigelow & Holmes Inc., distributed under the
license in GO_FONT_LICENSE.txt. FPDF_LICENSE.txt and X_IMAGE_LICENSE.txt contain
the full dependency notices. All three notices are compiled into the binary,
included in the generated PDF metadata, and copied into the runtime Docker image.

`docker build -t chugun-backend:presentation ./backend` builds the standalone
image. Compose continues to use its existing Go image and mounted source; changing
the shared Compose setup is outside this Backend-only change.
`TestPresentationPDFStructureAndCyrillic` checks five pages, xref offsets, embedded
font, Unicode map and Cyrillic in page content. TEST_PRESENTATION_PDF optionally
writes the test artifact for independent PDF parser checks.

Request limits: 1–50 tasks, key <=128, summary <=500, description <=8000 characters.
Response limits: title <=200, <=10 bullets per slide, bullet text <=1000 characters.
Input errors return 422; invalid AI output 502; unavailable AI 503; timeout 504.
The five-slide renderer reduces font size down to 7pt. Content which still cannot
fit returns an error rather than clipping source facts or adding extra slides.

## Verification on 13.11.2026

- Go 1.22.12: `go test ./...` and `go vet ./...` passed.
- Full tests against an isolated PostgreSQL 17 database passed; Go statement
  coverage was 84.9% after direct routing (`backend/coverage.out`, ignored by Git).
- `docker build -t chugun-backend:presentation ./backend` passed.
- Independent `pypdf 5.9.0` strict parsing of the generated test artifact passed:
  five pages, Cyrillic extracted, DEMO-101–103 present, DEMO-104/105 absent.
  This parser is a temporary verification tool, not an application dependency.
- Real AI generation and Frontend attachment display require the other team's
  implementation; this is not a claim of full Compose end-to-end completion.
- Go client compatibility was verified against the real FastAPI endpoint from
  AI commit `9a03b41` in Docker, replacing only the LLM provider with a deterministic
  test double. `TestPresentationAIContract` passed and produced a valid PDF.
  To repeat, set TEST_PRESENTATION_AI_URL and TEST_PRESENTATION_AI_TOKEN to the
  isolated test endpoint and run the Go suite. Never use real employee data.
- Regression tests reject cropped sentences/negations and incomplete 50-task
  output; 502 incomplete_presentation is preserved across the AI/Go boundary.

## Browser integration check while routing awaits agreement

An isolated copy of `origin/feat/frontend-presentation` was built and tested:
lint/build passed, 35 tests passed, line coverage 98.52%.
Headless Chrome exercised the real Go Backend and Postgres through a same-origin
local proxy. A fake AI HTTP server supplied `create_presentation` and grounded
slide JSON; this does not demonstrate the missing tool in the real AI agent.
At viewport widths 1280, 390 and 320px the PDF card had no horizontal overflow,
downloaded a real Go-rendered PDF, sent the HttpOnly `/api` cookie, rejected a
foreign session with 404, and preserved ordinary responses without a new PDF card.
The downloaded PDF passed strict independent parsing: five pages, Cyrillic,
DEMO-101–103 present, DEMO-104/105 absent.

Frontend issue reproduced: after the artifact expires, Backend correctly returns
404 but the plain download link stays active and the UI shows no alert or path to
request another presentation. Browser download is canceled. Frontend needs
download error handling; green unit tests do not currently cover this case.
Screenshots and raw results are local test artifacts under `backend/tmp/`.
The externally shown mock preview remains separate; these results do not claim
real LLM generation or a completed full Compose end-to-end acceptance.
