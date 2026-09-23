package incident

import (
	"time"

	"github.com/google/uuid"
)

type Verdict string

const (
	FeedbackHelpful    Verdict = "helpful"
	FeedbackIncorrect  Verdict = "incorrect"
	FeedbackIncomplete Verdict = "incomplete"
)

type LessonStatus string

const (
	LessonPending  LessonStatus = "pending"
	LessonActive   LessonStatus = "active"
	LessonDisabled LessonStatus = "disabled"
)

type LessonDraft struct {
	Condition   string   `json:"condition"`
	Check       string   `json:"check"`
	Rationale   string   `json:"rationale"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type Reflection struct {
	Summary string       `json:"summary"`
	Lesson  *LessonDraft `json:"lesson"`
}

type Feedback struct {
	ID         uuid.UUID   `json:"id"`
	RunID      uuid.UUID   `json:"run_id"`
	Verdict    Verdict     `json:"verdict"`
	Note       string      `json:"note"`
	CreatedAt  time.Time   `json:"created_at"`
	Reflection *Reflection `json:"reflection,omitempty"`
	Metadata   Metadata    `json:"metadata"`
	Error      string      `json:"error,omitempty"`
}

type Lesson struct {
	ID         uuid.UUID    `json:"id"`
	FeedbackID uuid.UUID    `json:"feedback_id"`
	RunID      uuid.UUID    `json:"run_id"`
	Mode       string       `json:"mode"`
	Scope      Scope        `json:"scope"`
	Draft      LessonDraft  `json:"draft"`
	Status     LessonStatus `json:"status"`
	CreatedAt  time.Time    `json:"created_at"`
}

type MemoryRule struct {
	ID        uuid.UUID `json:"id"`
	Condition string    `json:"condition"`
	Check     string    `json:"check"`
	Rationale string    `json:"rationale"`
}
