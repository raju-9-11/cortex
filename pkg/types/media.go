package types

import "time"

// =============================================================================
// Voice Types — Speech-to-Text and Text-to-Speech
// =============================================================================

// TranscribeResponse is the response for POST /api/voice/transcribe.
type TranscribeResponse struct {
	Text            string              `json:"text"`
	Language        string              `json:"language"`
	DurationSeconds float64            `json:"duration_seconds"`
	Segments        []TranscribeSegment `json:"segments"`
}

// TranscribeSegment represents a timed segment within a transcription result.
type TranscribeSegment struct {
	ID         int     `json:"id"`
	Start      float64 `json:"start"`       // Seconds from start of audio
	End        float64 `json:"end"`         // Seconds from start of audio
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`  // 0.0–1.0
}

// SynthesizeRequest is the JSON request body for POST /api/voice/synthesize.
type SynthesizeRequest struct {
	Text   string  `json:"text"`              // Required. Max 4096 chars.
	Voice  string  `json:"voice,omitempty"`   // Default: "nova"
	Speed  float64 `json:"speed,omitempty"`   // 0.25–4.0, default 1.0
	Format string  `json:"format,omitempty"`  // "mp3" (default), "opus", "aac", "flac", "wav", "pcm"
}

// SynthesizeRequest does not have a JSON response type — the handler writes
// raw audio bytes with Content-Type set to the appropriate MIME type.

// Voice ID constants (maps to upstream TTS provider voices).
const (
	VoiceAlloy   = "alloy"
	VoiceEcho    = "echo"
	VoiceFable   = "fable"
	VoiceOnyx    = "onyx"
	VoiceNova    = "nova"
	VoiceShimmer = "shimmer"
)

// Audio format constants.
const (
	AudioFormatMP3  = "mp3"
	AudioFormatOpus = "opus"
	AudioFormatAAC  = "aac"
	AudioFormatFLAC = "flac"
	AudioFormatWAV  = "wav"
	AudioFormatPCM  = "pcm"
)

// AudioFormatToMIME maps output format names to MIME types.
var AudioFormatToMIME = map[string]string{
	AudioFormatMP3:  "audio/mpeg",
	AudioFormatOpus: "audio/opus",
	AudioFormatAAC:  "audio/aac",
	AudioFormatFLAC: "audio/flac",
	AudioFormatWAV:  "audio/wav",
	AudioFormatPCM:  "audio/L16",
}

// Transcription model constants.
const (
	TranscriptionModelWhisper1 = "whisper-1"
)

// =============================================================================
// Media Types — File Upload, Retrieval, and Deletion
// =============================================================================

// MediaType categorizes uploaded media.
type MediaType string

const (
	MediaTypeImage    MediaType = "image"
	MediaTypeAudio    MediaType = "audio"
	MediaTypeVideo    MediaType = "video"
	MediaTypeDocument MediaType = "document"
)

// Media represents a stored media file.
// Used as the response for POST /api/media/upload and internally as the DB row.
type Media struct {
	ID          string    `json:"id"`                    // Prefixed: "med_..."
	Type        MediaType `json:"type"`                  // Derived from MIME: image, audio, video, document
	MIMEType    string    `json:"mime_type"`             // Detected MIME type, e.g. "image/png"
	SizeBytes   int64     `json:"size_bytes"`            // File size in bytes
	URL         string    `json:"url"`                   // Relative URL: "/api/media/{id}"
	Filename    string    `json:"filename,omitempty"`    // Original upload filename
	Description string    `json:"description,omitempty"` // Alt-text / accessibility description
	SessionID   string    `json:"session_id,omitempty"`  // Associated session (for lifecycle cleanup)
	CreatedAt   time.Time `json:"created_at"`
}

// Media size limits.
const (
	MaxMediaUploadBytes  = 50 * 1024 * 1024 // 50 MB
	MaxAudioUploadBytes  = 25 * 1024 * 1024 // 25 MB (for transcription)
)

// Allowed MIME type prefixes for media upload.
var AllowedMediaMIMEPrefixes = []string{
	"image/",
	"audio/",
	"video/",
}

// Additional allowed exact MIME types.
var AllowedMediaMIMEExact = []string{
	"application/pdf",
}

// =============================================================================
// Video Generation Types — Async Job-Based Workflow
// =============================================================================

// VideoJobStatus represents the lifecycle state of a generation job.
type VideoJobStatus string

const (
	VideoJobStatusQueued     VideoJobStatus = "queued"
	VideoJobStatusProcessing VideoJobStatus = "processing"
	VideoJobStatusCompleted  VideoJobStatus = "completed"
	VideoJobStatusFailed     VideoJobStatus = "failed"
	VideoJobStatusCancelled  VideoJobStatus = "cancelled"
)

// VideoGenerateRequest is the JSON request body for POST /api/video/generate.
type VideoGenerateRequest struct {
	Prompt          string `json:"prompt"`                       // Required. Text description of desired video.
	Model           string `json:"model,omitempty"`              // Default: "stable-video-diffusion"
	DurationSeconds int    `json:"duration_seconds,omitempty"`   // 2–10, default 4
	Resolution      string `json:"resolution,omitempty"`         // e.g. "1280x720" (default)
	Style           string `json:"style,omitempty"`              // "cinematic", "animated", "realistic", "artistic"
	NegativePrompt  string `json:"negative_prompt,omitempty"`    // What to avoid in generation
	SessionID       string `json:"session_id,omitempty"`         // Associate with session for context
}

// VideoGenerateResponse is returned by POST /api/video/generate (202 Accepted).
type VideoGenerateResponse struct {
	JobID            string    `json:"job_id"`                       // Prefixed: "vjob_..."
	Status           VideoJobStatus `json:"status"`                  // Always "queued" on creation
	CreatedAt        time.Time `json:"created_at"`
	EstimatedSeconds int       `json:"estimated_seconds,omitempty"` // Rough ETA
}

// VideoJobResponse is returned by GET /api/video/jobs/{id}.
type VideoJobResponse struct {
	JobID                    string          `json:"job_id"`
	Status                   VideoJobStatus  `json:"status"`
	Progress                 float64         `json:"progress"`                            // 0.0–1.0
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
	EstimatedSecondsRemaining *int           `json:"estimated_seconds_remaining,omitempty"`
	Result                   *VideoJobResult `json:"result,omitempty"`                    // Present when status == completed
	Error                    *APIError       `json:"error,omitempty"`                     // Present when status == failed
}

// VideoJobResult contains the output of a completed generation job.
type VideoJobResult struct {
	MediaID         string `json:"media_id"`
	URL             string `json:"url"`              // "/api/media/{media_id}"
	DurationSeconds int    `json:"duration_seconds"`
	Resolution      string `json:"resolution"`
	SizeBytes       int64  `json:"size_bytes"`
	MIMEType        string `json:"mime_type"`         // e.g. "video/mp4"
}

// VideoJobListResponse is returned by GET /api/video/jobs.
type VideoJobListResponse struct {
	Jobs   []VideoJobResponse `json:"jobs"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// Video generation defaults and constraints.
const (
	VideoDefaultModel      = "stable-video-diffusion"
	VideoDefaultDuration   = 4
	VideoMinDuration       = 2
	VideoMaxDuration       = 10
	VideoDefaultResolution = "1280x720"
)

// Valid video style presets.
var VideoStylePresets = []string{
	"cinematic",
	"animated",
	"realistic",
	"artistic",
}

// =============================================================================
// Extended Message Types — Attachments
// =============================================================================

// MessageAttachment links a media file to a message.
// Stored in the message's metadata; rendered inline in GET session responses.
type MessageAttachment struct {
	MediaID         string  `json:"media_id"`
	Type            MediaType `json:"type"`                          // "image", "audio", "video", "document"
	MIMEType        string  `json:"mime_type"`
	URL             string  `json:"url"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`     // For audio/video
	Description     string  `json:"description,omitempty"`          // Alt-text for accessibility
}

// SendMessageMultipartFields defines the multipart form fields accepted by
// POST /api/sessions/{id}/messages when Content-Type is multipart/form-data.
//
// This is not a JSON-parsed struct — it documents the form field names that
// the handler extracts from the multipart request.
//
//	audio           binary   required   Audio file to transcribe
//	language        string   optional   BCP-47 language hint
//	respond_as_voice bool    optional   If "true", response includes TTS audio
type SendMessageMultipartFields struct {
	// Documented via comments; actual parsing is manual in the handler.
}

// Extended SendMessageRequest adds optional attachments to the existing request.
// The base fields (Content, Role, Stream, ParentID) are unchanged.
// Note: this extends the existing SendMessageRequest — see message.go.
// Implementation approach: add Attachments field to SendMessageRequest.

// MessageWithAttachments extends Message for responses that include attachments.
// Used in SessionDetailResponse.Messages when attachments are present.
type MessageWithAttachments struct {
	Message                                   // Embed the base message
	Attachments []MessageAttachment `json:"attachments,omitempty"` // Media references
	VoiceURL    string              `json:"voice_url,omitempty"`   // TTS audio URL (assistant only)
}

// VoiceSSEEvent is sent as an SSE event of type "voice" during streaming
// responses when respond_as_voice=true.
type VoiceSSEEvent struct {
	AudioURL        string  `json:"audio_url"`
	DurationSeconds float64 `json:"duration_seconds"`
	MIMEType        string  `json:"mime_type"`
}

// =============================================================================
// WebSocket Real-Time Voice Types
// =============================================================================

// Voice WebSocket event types (distinct namespace from inference WS events).
const (
	// Client → Server
	WSVoiceInputAudioStart WSEventType = "voice.input_audio_start"
	WSVoiceInputAudioEnd   WSEventType = "voice.input_audio_end"
	WSVoiceCancel          WSEventType = "voice.cancel"
	WSVoiceConfig          WSEventType = "voice.config"
	WSVoiceVADSpeechStart  WSEventType = "voice.vad_speech_start"
	WSVoiceVADSpeechEnd    WSEventType = "voice.vad_speech_end"

	// Server → Client
	WSVoiceTranscription   WSEventType = "voice.transcription"
	WSVoiceResponseText    WSEventType = "voice.response_text"
	WSVoiceAudioStart      WSEventType = "voice.audio_start"
	WSVoiceAudioEnd        WSEventType = "voice.audio_end"
	WSVoiceError           WSEventType = "voice.error"
	WSVoiceSessionUpdated  WSEventType = "voice.session_updated"
)

// VoiceConfigPayload is sent by the client to update voice stream settings.
type VoiceConfigPayload struct {
	Voice    string  `json:"voice,omitempty"`    // TTS voice ID
	Speed    float64 `json:"speed,omitempty"`    // TTS speed multiplier
	Language string  `json:"language,omitempty"` // STT language hint
}

// VoiceTranscriptionPayload is sent by the server with STT results.
type VoiceTranscriptionPayload struct {
	Text       string  `json:"text"`
	IsFinal    bool    `json:"is_final"`
	Confidence float64 `json:"confidence"`
}

// VoiceResponseTextPayload is sent by the server with streamed LLM text.
type VoiceResponseTextPayload struct {
	Delta     string `json:"delta"`
	MessageID string `json:"message_id"`
}

// VoiceAudioStartPayload signals the beginning of a TTS audio stream.
type VoiceAudioStartPayload struct {
	MIMEType   string `json:"mime_type"`    // "audio/pcm"
	SampleRate int    `json:"sample_rate"`  // 24000
	MessageID  string `json:"message_id"`
}

// VoiceAudioEndPayload signals the end of a TTS audio stream.
type VoiceAudioEndPayload struct {
	MessageID       string  `json:"message_id"`
	DurationSeconds float64 `json:"duration_seconds"`
}

// VoiceErrorPayload wraps an APIError in a voice WS context.
type VoiceErrorPayload struct {
	Code    string `json:"code"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// VoiceSessionUpdatedPayload notifies the client that messages were persisted.
type VoiceSessionUpdatedPayload struct {
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"`
}

// Voice WebSocket close codes (application-defined, 4000–4999 range).
const (
	WSCloseSessionNotFound = 4000
	WSCloseQuotaExceeded   = 4001
	WSCloseServerOverloaded = 4002
)

// Voice WebSocket connection constraints.
const (
	VoiceWSPingInterval     = 30 * time.Second
	VoiceWSIdleTimeout      = 5 * time.Minute
	VoiceWSMaxDuration      = 30 * time.Minute
	VoiceWSAudioSampleRate  = 24000 // Hz
	VoiceWSAudioBitDepth    = 16    // bits
	VoiceWSAudioChannels    = 1     // mono
)

// =============================================================================
// New Error Codes — Voice, Media, and Video
// =============================================================================

const (
	ErrCodeFileTooLarge       = "file_too_large"
	ErrCodeUnsupportedFormat  = "unsupported_format"
	ErrCodeTranscriptionFailed = "transcription_failed"
	ErrCodeSynthesisFailed    = "synthesis_failed"
	ErrCodeGenerationFailed   = "generation_failed"
	ErrCodeQuotaExceeded      = "quota_exceeded"
	ErrCodeProviderNoVision   = "provider_no_vision"
	ErrCodeMediaNotFound      = "media_not_found"
	ErrCodeJobNotFound        = "job_not_found"
	ErrCodeInvalidAudioFormat = "invalid_audio_format"
)
