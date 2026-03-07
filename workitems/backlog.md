# Backlog — Medium, Low & Feature Work

> **Scope:** Items not in Sprint 1. Ordered by theme, then priority within each theme.
> **Source:** Consolidated findings from Architecture, Design, LLM Engine, and Product reviews.

---

## 🟡 Medium Priority — Code Quality & Robustness

### WI-100: Add test coverage for OpenAI provider
- **Files:** `internal/inference/openai.go` (new `openai_test.go`)
- **Problem:** Zero tests for the OpenAI provider. Streaming, tool calls, error handling, and token counting are all untested.
- **Scope:** HTTP mock server testing Complete(), StreamChat(), ListModels(), error responses, rate limiting, malformed JSON.

### WI-101: Add test coverage for SSE pipeline
- **Files:** `internal/streaming/sse.go` (new `sse_test.go`)
- **Problem:** The SSE pipeline has no tests. Backpressure, stall detection, event formatting, and error propagation are untested.
- **Scope:** Test all 8 event types, client disconnect handling, timeout behavior, malformed events.

### WI-102: Add test coverage for registry
- **Files:** `internal/inference/registry.go` (new `registry_test.go`)
- **Problem:** Model resolution (prefix, catalog, default, fallback) has no tests.
- **Scope:** Test Register, Resolve (all paths), SetDefault, RefreshModelMap, ListAllModels, concurrent access.

### WI-103: Make ProviderRegistry an interface
- **Files:** `internal/inference/registry.go`, `internal/api/*.go`, `internal/server/server.go`
- **Problem:** `ProviderRegistry` is a concrete struct, making it impossible to mock in handler tests without real providers.
- **Fix:** Extract a `Registry` interface with `Resolve()`, `ListAllModels()`, `Providers()`. Handlers accept the interface.

### WI-104: Extract `accumulatingProvider` from handlers
- **Files:** `internal/api/handlers_sessions.go` → new `internal/inference/accumulator.go`
- **Problem:** The `accumulatingProvider` wrapper is defined inside the API handlers package. It's an inference concern, not an API concern.
- **Fix:** Move to `internal/inference/` as a reusable decorator.

### WI-105: Validate ChatCompletionRequest fields
- **Files:** `internal/api/handlers_chat.go`
- **Problem:** No validation on `messages` (could be empty), `temperature` (could be negative), `max_tokens` (could be zero or enormous).
- **Fix:** Validate: messages non-empty, valid roles, temperature 0-2, max_tokens within bounds.

### WI-106: Add database CHECK constraints
- **Files:** `internal/store/migrations/` (new migration)
- **Problem:** No constraints on `sessions.status`, `messages.role`. Invalid values can be inserted.
- **Fix:** Add `CHECK(status IN ('active','idle','archived'))`, `CHECK(role IN ('system','user','assistant','tool'))`.

### WI-107: Add message `created_at` index
- **Files:** `internal/store/migrations/` (new migration)
- **Problem:** `GetMessages` queries by `session_id + is_active + created_at` but there's no composite index.
- **Fix:** `CREATE INDEX idx_messages_session_active_created ON messages(session_id, is_active, created_at)`.

### WI-108: Handle `time.Parse` errors in DB scan helpers
- **Files:** `internal/store/sqlite.go`
- **Problem:** All timestamp parsing uses `time.Parse()` with errors silently discarded (zero time used on failure).
- **Fix:** Log parse errors. Consider storing as Unix timestamps instead of ISO strings.

### WI-109: Remove mock providers from production startup
- **Files:** `cmd/forge/main.go:79-86`
- **Problem:** When no real providers are configured, mock providers are registered and serve fake responses. This can confuse users.
- **Fix:** Only register mocks if `FORGE_DEV=true`. Otherwise, start with zero providers and return clear errors.

### WI-110: Add `Probe()` to InferenceProvider interface
- **Files:** `internal/inference/openai.go`, `internal/inference/registry.go`
- **Problem:** Only OllamaProvider has `Probe()`. The health endpoint can't check OpenAI-compatible provider connectivity.
- **Fix:** Add `Probe(ctx context.Context) bool` to `InferenceProvider` interface. Implement for OpenAI (HEAD request to base URL).

### WI-111: Add per-session inference lock
- **Files:** `internal/session/manager.go` or `internal/api/handlers_sessions.go`
- **Problem:** Two concurrent `POST /api/sessions/{id}/messages` can interleave, producing garbled conversation history.
- **Fix:** `sync.Map` of session ID → mutex. Lock during message send. Return `409 Conflict` if already in progress.

### WI-112: Improve token counting accuracy
- **Files:** `internal/inference/openai.go`, new `internal/inference/tokenizer.go`
- **Problem:** `len(content)/4` underestimates CJK by 50-75% and overestimates code/punctuation.
- **Fix:** Integrate `tiktoken-go` for OpenAI models. For Ollama, use `/api/tokenize` endpoint. Fall back to `len/4` with logged warning.

---

## 🟢 Low Priority — Polish & Nice-to-Have

### WI-200: Consistent response envelopes
- **Files:** `internal/api/handlers_chat.go`, `internal/api/handlers_sessions.go`
- **Problem:** Some endpoints return bare objects, others use `{data: [...]}` wrappers.
- **Fix:** Standardize: collections return `{data: [...], total, has_more}`, singles return the object directly (OpenAI convention).

### WI-201: Add `GET /api/config` endpoint
- **Files:** `internal/api/` (new handler), `pkg/types/health.go` (ConfigResponse already defined)
- **Problem:** `ConfigResponse` type exists in pkg/types but no handler serves it.
- **Fix:** Implement handler returning non-sensitive config (providers, default model, limits). Never expose API keys.

### WI-202: Add `GET /api/providers` endpoint
- **Files:** `internal/api/` (new handler), `pkg/types/provider.go` (ProviderInfo already defined)
- **Problem:** `ProviderInfo` type exists but no endpoint lists available providers.
- **Fix:** List providers with name, status, model count. Never expose API keys (type already has `json:"-"` tag).

### WI-203: Content-Type enforcement on all mutation endpoints
- **Files:** `internal/api/handlers_sessions.go`
- **Problem:** POST/PATCH endpoints don't validate Content-Type. Missing header is accepted.
- **Fix:** Chi middleware or per-handler check requiring `application/json` on POST/PATCH/PUT.

### WI-204: Add retry jitter randomization
- **Files:** `internal/inference/retry.go`
- **Problem:** Retry backoff uses fixed exponential intervals. Multiple failing requests retry in lockstep.
- **Fix:** Add ±25% random jitter to backoff intervals.

### WI-205: Replace hardcoded user ID
- **Files:** `internal/session/manager.go`
- **Problem:** User ID is hardcoded (e.g., `"default"` or empty). Multi-user support will require refactoring.
- **Fix:** Accept user ID from auth middleware context. Keep single-user default for no-auth mode.

### WI-206: Add `Vary: Origin` header to CORS
- **Files:** `internal/server/server.go`
- **Problem:** Missing `Vary: Origin` causes incorrect caching when multiple origins are configured.
- **Fix:** Add `Vary: Origin` when reflecting a specific origin (not `*`).

---

## 📦 Feature Backlog — Future Phases

### FB-001: Frontend — Chat UI (Phase B)
- **Priority:** Next major milestone
- **Scope:** React 19 + Vite + Tailwind CSS v4, embedded via `go:embed`
- **Components:**
  - Message thread with streaming markdown renderer
  - Conversation sidebar (create, rename, delete, search)
  - Model selector dropdown
  - System prompt editor
  - Settings page (provider config, theme)
  - Copy / regenerate / edit message buttons
  - Dark/light theme toggle
  - Keyboard shortcuts (Cmd+N new chat, Cmd+K search, Enter send)
- **Acceptance:** User downloads binary, runs it, opens browser, and is chatting within 60 seconds.

### FB-002: Anthropic Provider
- **Priority:** High (different API format from OpenAI)
- **Scope:** Dedicated provider for Claude models. Messages API format, system prompt as top-level field, streaming via SSE with `content_block_delta` events.

### FB-003: Gemini Provider
- **Priority:** Medium
- **Scope:** Google Gemini API. Different content structure (`parts` array), safety settings, grounding.

### FB-004: Context Window Manager
- **Priority:** High (after WI-007 quick fix)
- **Scope:** Proper token budget system. Per-model context limits. Strategy selection (truncate oldest, summarize, sliding window). Token counting integration. Budget visualization for Inspector UI.

### FB-005: Compaction Engine
- **Priority:** Medium
- **Scope:** Rolling summarization for long conversations. Summarize older messages into a condensed system message. Preserve key facts, decisions, and code snippets.

### FB-006: Tool Sandbox
- **Priority:** Medium
- **Scope:** Pause-Execute-Resume tool execution loop. Sandboxed command execution. User approval flow. Tool result injection into conversation.

### FB-007: Event Bus + WebSocket
- **Priority:** Low (needed for Inspector)
- **Scope:** Pub/sub event bus for internal events (inference start/end, token usage, tool execution). WebSocket endpoint for real-time event streaming to Inspector UI.

### FB-008: Inspector UI
- **Priority:** Low
- **Scope:** Token visualization, raw context viewer, event stream timeline, provider request/response viewer. Debug tool for prompt engineering.

### FB-009: PostgreSQL Adapter
- **Priority:** Low (SQLite handles most use cases)
- **Scope:** Implement `Store` interface for PostgreSQL. Connection pooling. Schema migrations in SQL dialect. Use when `DATABASE_URL` env var is set.

### FB-010: Conversation Branching
- **Priority:** Low
- **Scope:** Tree-based message structure. Branch from any message. Navigate branches in UI. Requires schema migration from flat list to parent_id tree.

### FB-011: CI/CD Pipeline
- **Priority:** Medium
- **Scope:** GitHub Actions workflow: `go test -race ./...`, `go vet`, `golangci-lint`. Goreleaser for cross-platform binary builds. Docker image. Homebrew tap.

### FB-012: Rate Limiting
- **Priority:** Medium (for multi-user deployments)
- **Scope:** Per-IP and per-API-key rate limiting. `429 Too Slow` responses with `Retry-After` header. Configurable via env vars. Use `golang.org/x/time/rate`.

---

---

## 🎙️ Voice & Video Features

> **Source:** Consolidated from Product Manager, Architect, and Design agent specs.
> **Phased approach:** Foundation → Analysis → Generation → Real-time.

### Phase V1 — Voice Foundation + Vision (Image)

#### VV-001: Media storage layer (`internal/media/`)
- **Priority:** High — prerequisite for all voice/video features
- **Scope:** New `internal/media/` package with `MediaStore` interface
- **Components:**
  - File storage on local filesystem (`./data/media/`)
  - SQLite metadata table: `media_attachments` (id `med_<uuid>`, type, mime_type, size_bytes, path, checksum, created_at)
  - `Upload(ctx, io.Reader, metadata) → MediaRecord` — streams to disk, records metadata
  - `Get(ctx, id) → MediaRecord` — metadata lookup
  - `Open(ctx, id) → io.ReadCloser` — file content
  - `Delete(ctx, id)` — remove file + metadata
  - `LinkToMessage(ctx, mediaID, messageID)` — association table
- **Config:** `FORGE_MEDIA_DIR` (default `./data/media/`), `FORGE_MAX_UPLOAD_BYTES` (default 50MB)
- **Acceptance:** Upload a file via API, retrieve it, delete it. Files survive server restart.

#### VV-002: Media upload/serve API endpoints
- **Priority:** High — prerequisite for attachments
- **Files:** New `internal/api/handlers_media.go`
- **Endpoints:**
  - `POST /api/media/upload` — multipart file upload → `{id, type, mime_type, size, url}`
  - `GET /api/media/{id}` — serve file with correct Content-Type, support Range requests for video
  - `DELETE /api/media/{id}` — remove media
- **Validation:** Max file size (50MB), allowed MIME types (image/jpeg, image/png, image/webp, audio/mp3, audio/wav, audio/webm, video/mp4, video/webm)
- **Error codes:** `file_too_large`, `unsupported_format`
- **Acceptance:** Upload image/audio/video via curl, retrieve by ID, delete.

#### VV-003: Extend message model with attachments
- **Priority:** High — enables multimodal messages
- **Files:** `pkg/types/message.go`, `internal/api/handlers_sessions.go`, `internal/store/store.go`, `internal/store/sqlite.go`
- **Changes:**
  - Add `Attachments []MessageAttachment` to `SendMessageRequest` (input: `media_id` only)
  - Add `Attachments []MessageAttachment` to message responses (output: full metadata + url)
  - `MessageAttachment` struct: `{media_id, type, mime_type, url, description}`
  - New join table `message_attachments` (message_id, media_id, position)
  - `description` field serves as alt-text for accessibility
- **Backward compatible:** Existing clients that don't send attachments are unaffected.
- **Acceptance:** Create message with attachment, retrieve session shows attachment metadata.

#### VV-004: Speech-to-text endpoint (OpenAI Whisper)
- **Priority:** High — core voice input
- **Files:** New `internal/voice/provider.go`, `internal/voice/openai_stt.go`, new `internal/api/handlers_voice.go`
- **Interface:**
  ```go
  type STTProvider interface {
      Transcribe(ctx context.Context, audio io.Reader, opts STTOptions) (*Transcription, error)
      SupportedFormats() []string
  }
  ```
- **Endpoint:** `POST /api/voice/transcribe` — multipart (file + model + language?) → `{text, language, duration, segments[]}`
- **Also:** `POST /v1/audio/transcriptions` (OpenAI-compatible alias)
- **Provider:** OpenAI Whisper API (uses existing `OPENAI_API_KEY`)
- **Limits:** Max 25MB audio, formats: mp3, wav, m4a, webm, ogg
- **Acceptance:** Upload audio file, receive accurate transcription within 10 seconds.

#### VV-005: Text-to-speech endpoint (OpenAI TTS)
- **Priority:** High — core voice output
- **Files:** New `internal/voice/openai_tts.go`, extend `internal/api/handlers_voice.go`
- **Interface:**
  ```go
  type TTSProvider interface {
      Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error)
      AvailableVoices() []Voice
  }
  ```
- **Endpoint:** `POST /api/voice/synthesize` — JSON `{text, voice, speed, format}` → binary audio stream (Content-Type: audio/mpeg)
- **Also:** `POST /v1/audio/speech` (OpenAI-compatible alias)
- **Provider:** OpenAI TTS API (voices: alloy, nova, echo, fable, onyx, shimmer)
- **Limits:** Max 4096 chars input. Formats: mp3, wav, opus
- **Response:** Chunked transfer encoding for streaming playback. `X-Audio-Duration` header.
- **Acceptance:** Send text, receive playable MP3 audio stream.

#### VV-006: Vision support in chat completions
- **Priority:** High — image analysis
- **Files:** `pkg/types/`, `internal/inference/openai.go`, `internal/inference/ollama.go`, `internal/inference/registry.go`
- **Changes:**
  - Extend `ChatCompletionRequest.Messages[].Content` to accept `string | []ContentPart`
  - `ContentPart` struct: `{type: "text"|"image_url", text?, image_url?: {url, detail?}}`
  - OpenAI provider: pass content parts as-is (native format)
  - Ollama provider: translate to `images` field (extract base64 from data URIs)
  - Registry: add `SupportsVision() bool` capability flag to providers
  - Route multimodal requests only to vision-capable models; return 400 otherwise
- **Supported models:** GPT-4o, GPT-4V (OpenAI), LLaVA, llama3.2-vision (Ollama)
- **Acceptance:** Send image + question to GPT-4o, get description back. Same with LLaVA via Ollama.

#### VV-007: Schema migration 002_media.sql
- **Priority:** High — prerequisite for VV-001 through VV-003
- **Files:** `internal/store/migrations/002_media.sql`
- **Tables:**
  ```sql
  CREATE TABLE media (
      id TEXT PRIMARY KEY,
      type TEXT NOT NULL CHECK(type IN ('image','audio','video')),
      mime_type TEXT NOT NULL,
      filename TEXT,
      size_bytes INTEGER NOT NULL,
      path TEXT NOT NULL,
      checksum TEXT,
      description TEXT,
      created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now'))
  );

  CREATE TABLE message_media (
      message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
      media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
      position INTEGER NOT NULL DEFAULT 0,
      PRIMARY KEY (message_id, media_id)
  );

  CREATE TABLE generation_jobs (
      id TEXT PRIMARY KEY,
      session_id TEXT REFERENCES sessions(id) ON DELETE SET NULL,
      type TEXT NOT NULL CHECK(type IN ('video','image')),
      provider TEXT NOT NULL,
      model TEXT,
      prompt TEXT NOT NULL,
      config_json TEXT,
      status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','processing','completed','failed','cancelled')),
      progress REAL DEFAULT 0.0,
      result_media_id TEXT REFERENCES media(id),
      error TEXT,
      created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
      updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now')),
      completed_at TEXT
  );

  CREATE INDEX idx_media_type ON media(type);
  CREATE INDEX idx_message_media_message ON message_media(message_id);
  CREATE INDEX idx_generation_jobs_status ON generation_jobs(status);
  CREATE INDEX idx_generation_jobs_session ON generation_jobs(session_id);
  ```
- **Acceptance:** Migration runs cleanly on existing databases. No data loss.

### Phase V2 — Video Analysis + Provider Expansion

#### VV-008: Video frame extraction
- **Priority:** Medium
- **Files:** New `internal/video/frames.go`
- **Scope:**
  - `FrameExtractor` struct that shells out to `ffmpeg` (checked via `exec.LookPath`)
  - `HasFFmpeg() bool` — graceful degradation (if absent, only image uploads accepted)
  - `ExtractFrames(videoPath, opts) → []Frame` — uniform sampling (1/sec) or scene-change detection
  - Frames resized to max 1024px on longest edge, output as JPEG
  - EXIF data stripped for privacy
- **Config:** `FORGE_FFMPEG_PATH` (default: auto-detect)
- **Limits:** Max 60 seconds video, max 50MB, max 30 frames per extraction
- **Acceptance:** Upload 30-second MP4, get 10 keyframes as base64 JPEG.

#### VV-009: Video analysis API endpoint
- **Priority:** Medium — depends on VV-006 (vision) + VV-008 (frames)
- **Files:** Extend `internal/api/handlers_media.go`
- **Endpoint:** `POST /api/video/analyze` — multipart (video file + prompt + max_frames?) → extracted frames sent to vision model → text response
- **Also:** Convenience endpoint that combines frame extraction + vision chat in one call
- **Alternative flow:** `POST /v1/video/extract-frames` returns frames, client includes in `/v1/chat/completions`
- **Token cost:** Log estimated token cost (OpenAI: ~85 tokens per 512×512 tile). Display in response metadata.
- **Acceptance:** Upload a video, ask "what happens in this video?", get accurate description.

#### VV-010: Deepgram STT provider
- **Priority:** Medium
- **Files:** New `internal/voice/deepgram.go`
- **Scope:** Implement `STTProvider` for Deepgram API. REST-based transcription.
- **Config:** `DEEPGRAM_API_KEY`
- **Why:** Superior accuracy for noisy audio. Lower cost at scale. Needed for real-time STT (Phase V3).
- **Acceptance:** Transcribe same audio file via Deepgram and OpenAI, both return results.

#### VV-011: ElevenLabs TTS provider
- **Priority:** Medium
- **Files:** New `internal/voice/elevenlabs.go`
- **Scope:** Implement `TTSProvider` for ElevenLabs API. Streaming audio output.
- **Config:** `ELEVENLABS_API_KEY`
- **Why:** Premium voice quality, voice cloning capability. Differentiated from OpenAI TTS.
- **Acceptance:** Synthesize text with ElevenLabs voice, verify higher quality audio.

#### VV-012: Anthropic Claude Vision provider
- **Priority:** Medium — depends on VV-006
- **Files:** New `internal/inference/anthropic.go`
- **Scope:** Full Anthropic provider (chat + vision). Different API format: `source` blocks with `media_type` + `data`, system prompt as top-level field, `content_block_delta` streaming.
- **Config:** `ANTHROPIC_API_KEY` (already in config, unused)
- **Acceptance:** Send image to Claude, get analysis. Text chat also works.

#### VV-013: Google Gemini Vision provider
- **Priority:** Medium
- **Files:** New `internal/inference/gemini.go`
- **Scope:** Gemini provider (chat + native video understanding). `inlineData` format with `mimeType` + `data`. Native video analysis (no frame extraction needed).
- **Config:** `GEMINI_API_KEY`
- **Acceptance:** Send video directly to Gemini (no frame extraction), get analysis.

### Phase V3 — Video Generation

#### VV-014: Video generation job queue
- **Priority:** Medium — infrastructure for async gen
- **Files:** New `internal/video/generator.go`, `internal/video/poller.go`
- **Interface:**
  ```go
  type VideoGenerator interface {
      Submit(ctx context.Context, req VideoGenRequest) (jobID string, err error)
      Poll(ctx context.Context, jobID string) (*VideoGenStatus, error)
      Cancel(ctx context.Context, jobID string) error
      Download(ctx context.Context, jobID string) (io.ReadCloser, error)
  }
  ```
- **Background poller:** Single goroutine polls `generation_jobs` table every 5 seconds. Picks oldest `queued` → sets `processing` → calls provider → polls for completion → downloads result → saves to media store → sets `completed`.
- **Concurrency:** 1 active job at a time (configurable). Queue the rest.
- **Crash recovery:** On startup, resume polling for any `processing` jobs.
- **Acceptance:** Submit job, restart server, job still completes.

#### VV-015: Video generation API endpoints
- **Priority:** Medium — depends on VV-014
- **Files:** New `internal/api/handlers_video.go`
- **Endpoints:**
  - `POST /api/video/generate` — `{prompt, model, duration, resolution, reference_image?}` → 202 `{job_id, status, poll_url}`
  - `GET /api/video/jobs/{id}` — `{job_id, status, progress, video_url?, error?}`
  - `GET /api/video/jobs` — list jobs, filter by `?status=` and `?session_id=`
  - `DELETE /api/video/jobs/{id}` — cancel/delete
- **Error codes:** `generation_failed`, `quota_exceeded`, `content_policy_violation`
- **Acceptance:** Submit generation request, poll until complete, download video.

#### VV-016: OpenAI Sora video generation provider
- **Priority:** Medium — depends on VV-014
- **Files:** New `internal/video/sora.go`
- **Scope:** Implement `VideoGenerator` for OpenAI Sora API (or whatever the API surface is at implementation time).
- **Config:** Uses existing `OPENAI_API_KEY`
- **Note:** Video generation APIs are nascent and may change. Provider abstraction is critical.
- **Acceptance:** Generate a 5-second video from text prompt via Sora.

### Phase V4 — Real-time Voice

#### VV-017: WebSocket real-time voice streaming
- **Priority:** Low — requires Deepgram (VV-010) + solid STT/TTS foundation
- **Files:** New `internal/api/handlers_voice_ws.go`
- **Endpoint:** `WS /api/voice/stream?session_id=X`
- **Protocol:**
  - Client sends: binary audio frames (opus/webm chunks)
  - Server sends: JSON text frames (`{type: "transcription", text, is_final}`) + binary audio frames (TTS response)
  - Turn detection: VAD (voice activity detection) via energy threshold or provider-side
  - Interruption handling: client starts speaking → server stops TTS playback
- **Idle timeout:** 5 minutes no audio → close connection
- **Acceptance:** Open WebSocket, speak, receive live transcription + spoken AI response.

#### VV-018: Session-level voice settings
- **Priority:** Low — depends on VV-004 + VV-005
- **Files:** `pkg/types/session.go`, `internal/session/manager.go`
- **Changes:**
  - Add `VoiceSettings` to session: `{stt_enabled, tts_enabled, tts_voice, tts_speed, language}`
  - `PATCH /api/sessions/{id}` accepts `voice` field
  - When `tts_enabled`, chat responses automatically include synthesized audio URL
- **Acceptance:** Enable TTS on a session, send text message, response includes audio URL.

---

## 🔧 Voice & Video — Config Additions

| Variable | Default | Phase | Description |
|----------|---------|-------|-------------|
| `FORGE_MEDIA_DIR` | `./data/media/` | V1 | Media file storage directory |
| `FORGE_MAX_UPLOAD_BYTES` | `52428800` (50MB) | V1 | Max upload file size |
| `ELEVENLABS_API_KEY` | *(empty)* | V2 | ElevenLabs TTS API key |
| `DEEPGRAM_API_KEY` | *(empty)* | V2 | Deepgram STT API key |
| `GEMINI_API_KEY` | *(empty)* | V2 | Google Gemini API key |
| `FORGE_FFMPEG_PATH` | *(auto-detect)* | V2 | Path to ffmpeg binary |
| `FORGE_MAX_VIDEO_DURATION` | `60` | V2 | Max video duration (seconds) |
| `FORGE_MAX_FRAMES` | `30` | V2 | Max frames per video extraction |
| `FORGE_VIDEO_GEN_CONCURRENCY` | `1` | V3 | Max concurrent video generation jobs |

---

## 🏗️ Voice & Video — Dependency Graph

```
Phase V1 (parallel tracks):
  VV-007 (schema) ──┬──► VV-001 (media store) ──► VV-002 (media API) ──► VV-003 (attachments)
                    │
                    └──► VV-004 (STT) ─────────────────────────────────┐
                    └──► VV-005 (TTS) ─────────────────────────────────┤
                    └──► VV-006 (vision in chat) ──────────────────────┤
                                                                       ▼
Phase V2 (depends on V1):                                       V1 complete
  VV-008 (frame extraction) ──► VV-009 (video analysis)
  VV-010 (Deepgram) ─── parallel ─── VV-011 (ElevenLabs)
  VV-012 (Anthropic) ─── parallel ── VV-013 (Gemini)

Phase V3 (depends on V1 media):
  VV-014 (job queue) ──► VV-015 (gen API) ──► VV-016 (Sora provider)

Phase V4 (depends on V2):
  VV-010 (Deepgram) ──► VV-017 (WebSocket voice)
  VV-004 + VV-005 ────► VV-018 (session voice settings)
```

**Critical path:** VV-007 → VV-001 → VV-006 → VV-008 → VV-009 (video analysis end-to-end)
**Voice is fully parallel** — can be developed concurrently with video.

---

## Summary

| Category | Count | Sprint |
|----------|-------|--------|
| 🟡 Medium (code quality) | 13 | Sprint 2 |
| 🟢 Low (polish) | 7 | Sprint 3+ |
| 📦 Features (existing) | 12 | Phase B+ |
| 🎙️ Voice & Video | 18 | Phases V1–V4 |
| **Total** | **50** | |
