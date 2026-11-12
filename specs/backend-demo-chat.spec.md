# Server-side demo chat actions

This extends the intermediate JSON chat contract; it is not the production AI tool-call protocol.

## Frontend integration (PR #23)

Send every message to POST /api/chat with JSON {"message":"..."} and X-Request-ID.
Generate a key with crypto.randomUUID() for each new message. Keep the same key and
body for retries. Keep cookies (same-origin fetch does this by default); Nginx /api
proxy in PR #23 supports Set-Cookie. Do not classify intent or call write mock APIs
from the browser. Response remains {reply, sources?}, compatible with its Zod schema.

Backend creates an opaque HttpOnly SameSite=Strict demo_chat cookie with a 24h lifetime.
This cookie isolates synthetic conversations; it is NOT corporate authentication/IAM.
No user_id, role, recipient or confirmation supplied by an LLM is trusted here.

## Explicit demo commands

- «Оформи доступ к Jira» / «Создай заявку на доступ к Confluence» -> server draft.
- «Создай заявку на доступ» -> choose exactly Jira or Confluence -> draft.
- «Найди владельца сервиса billing» / «Найди владельца search» -> synthetic catalogue,
  recipient and exact message draft shown in reply.
- «подтверждаю» -> execute the pending draft once.
- «отмена» / «отмени» -> invalidate pending draft with no write.
- An unrelated message while a draft is pending invalidates it, never becomes an
  arbitrary service name. Changing recipient/text requires a new workflow (not yet exposed).
- Other questions with no pending action go to Python /chat. Negations and questions
  do not match the anchored command grammar and cannot directly execute tools.

## Persistence and retries

Conversation state and results live in demo_chat_sessions and demo_chat_results.
A transaction locks the session, writes the mock action, stores its response and clears
its draft atomically. Same session/key/body returns the saved response. Reusing a key
with different text returns 409 request_conflict. No automatic retry with a new key.
After a response/connection failure retry the same key: a committed result is recoverable.
After expiry a draft cannot execute. Expired session cookies naturally create a new dialogue.

Raw /api/mock/* endpoints remain synthetic adapter/test APIs, not authenticated agent tools.
They have no IAM or confirmation guarantees and must not be used as the browser action API.
The server catalogue and explicit commands are a demo fallback, not LLM intent detection.
Production trusted context, permissions and AI tool-call negotiation remain separate work.

## Verification

CI runs Postgres 16, sets TEST_DATABASE_URL, runs all Go tests and enforces >80% coverage.
The integration test covers cancellation, questions/negation, stale drafts, cross-session
confirmation, idempotent replay, conflicting keys, handler restart and expired drafts.
Use an isolated synthetic database: integration tests create demo records.
Without TEST_DATABASE_URL database integration tests are explicitly skipped.
