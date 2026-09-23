package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/incident"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxMemoryRules = 5

func (s *Store) AddFeedback(ctx context.Context, runID uuid.UUID, verdict incident.Verdict, note string) (incident.Feedback, error) {
	if verdict != incident.FeedbackHelpful && verdict != incident.FeedbackIncorrect && verdict != incident.FeedbackIncomplete {
		return incident.Feedback{}, fmt.Errorf("verdict must be helpful, incorrect or incomplete")
	}
	note = strings.TrimSpace(note)
	if note == "" || utf8.RuneCountInString(note) > 2000 {
		return incident.Feedback{}, fmt.Errorf("feedback note must contain 1..2000 characters")
	}
	var f incident.Feedback
	err := s.tx(ctx, func(q *db.Queries) error {
		row, err := q.InsertFeedback(ctx, db.InsertFeedbackParams{ID: uuid.New(), RunID: runID, Verdict: string(verdict), Note: note})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("feedback requires an existing completed investigation run")
		}
		if err != nil {
			return err
		}
		f, err = decodeFeedback(row)
		if err != nil {
			return err
		}
		r, err := q.GetRun(ctx, runID)
		if err != nil {
			return err
		}
		i, err := decodeIncident(q.GetIncident(ctx, r.IncidentID))
		if err != nil {
			return err
		}
		return appendTimeline(ctx, q, i, "feedback_added", f)
	})
	return f, err
}

func (s *Store) Feedback(ctx context.Context, id uuid.UUID) (incident.Feedback, error) {
	row, err := s.q.GetFeedback(ctx, id)
	if err != nil {
		return incident.Feedback{}, err
	}
	return decodeFeedback(row)
}

func decodeFeedback(row db.Feedback) (incident.Feedback, error) {
	f := incident.Feedback{ID: row.ID, RunID: row.RunID, Verdict: incident.Verdict(row.Verdict), Note: row.Note, CreatedAt: row.CreatedAt, Error: row.ErrorMessage}
	if row.Reflection != nil {
		if err := json.Unmarshal(row.Reflection, &f.Reflection); err != nil {
			return f, err
		}
	}
	if err := json.Unmarshal(row.Metadata, &f.Metadata); err != nil {
		return f, err
	}
	return f, nil
}

func (s *Store) SaveReflection(ctx context.Context, f incident.Feedback, result incident.Reflection, meta incident.Metadata) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(q *db.Queries) error {
		n, err := q.CompleteReflection(ctx, db.CompleteReflectionParams{ID: f.ID, Reflection: raw, Metadata: metadata})
		if err != nil || n == 0 {
			return err
		}
		r, err := q.GetRun(ctx, f.RunID)
		if err != nil {
			return err
		}
		i, err := decodeIncident(q.GetIncident(ctx, r.IncidentID))
		if err != nil {
			return err
		}
		if result.Lesson != nil {
			draft, err := json.Marshal(result.Lesson)
			if err != nil {
				return err
			}
			row, err := q.InsertLesson(ctx, db.InsertLessonParams{
				ID: uuid.New(), FeedbackID: f.ID, SourceRunID: r.ID, Mode: r.Mode,
				MerchantID: i.Scope.MerchantID, PaymentMethod: i.Scope.Method, Environment: i.Scope.Environment,
				SourceWindowStart: i.Scope.WindowStart, SourceWindowEnd: i.Scope.WindowEnd, Draft: draft,
			})
			if err != nil {
				return err
			}
			lesson, err := decodeLesson(row)
			if err != nil {
				return err
			}
			if err := appendTimeline(ctx, q, i, "lesson_proposed", lesson); err != nil {
				return err
			}
		}
		f.Reflection, f.Metadata, f.Error = &result, meta, ""
		return appendTimeline(ctx, q, i, "feedback_reflected", f)
	})
}

func (s *Store) ReflectionFailed(ctx context.Context, id uuid.UUID, cause error, meta incident.Metadata) error {
	metadata, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return s.q.ReflectionFailed(ctx, db.ReflectionFailedParams{ID: id, ErrorMessage: cause.Error(), Metadata: metadata})
}

func (s *Store) LessonForFeedback(ctx context.Context, id uuid.UUID) (*incident.Lesson, error) {
	row, err := s.q.LessonForFeedback(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l, err := decodeLesson(row)
	return &l, err
}

func (s *Store) Lesson(ctx context.Context, id uuid.UUID) (incident.Lesson, error) {
	row, err := s.q.GetLesson(ctx, id)
	if err != nil {
		return incident.Lesson{}, err
	}
	return decodeLesson(row)
}

func (s *Store) Lessons(ctx context.Context) ([]incident.Lesson, error) {
	rows, err := s.q.ListLessons(ctx)
	if err != nil {
		return nil, err
	}
	out := []incident.Lesson{}
	for _, row := range rows {
		l, err := decodeLesson(row)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func (s *Store) SetLessonStatus(ctx context.Context, id uuid.UUID, status incident.LessonStatus) (incident.Lesson, error) {
	var l incident.Lesson
	err := s.tx(ctx, func(q *db.Queries) error {
		var n int64
		var err error
		switch status {
		case incident.LessonActive:
			n, err = q.ActivateLesson(ctx, id)
		case incident.LessonDisabled:
			n, err = q.DisableLesson(ctx, id)
		default:
			return incident.ErrInvalidInput
		}
		if err != nil {
			return err
		}
		row, err := q.GetLesson(ctx, id)
		if err != nil {
			return err
		}
		l, err = decodeLesson(row)
		if err != nil {
			return err
		}
		if l.Status != status {
			return fmt.Errorf("cannot activate a disabled lesson")
		}
		if n == 0 {
			return nil
		}
		r, err := q.GetRun(ctx, l.RunID)
		if err != nil {
			return err
		}
		i, err := decodeIncident(q.GetIncident(ctx, r.IncidentID))
		if err != nil {
			return err
		}
		return appendTimeline(ctx, q, i, "lesson_"+string(status), l)
	})
	return l, err
}

func decodeLesson(row db.Lesson) (incident.Lesson, error) {
	l := incident.Lesson{
		ID: row.ID, FeedbackID: row.FeedbackID, RunID: row.SourceRunID, Mode: row.Mode,
		Scope:  incident.Scope{MerchantID: row.MerchantID, Method: row.PaymentMethod, Environment: row.Environment, WindowStart: row.SourceWindowStart, WindowEnd: row.SourceWindowEnd},
		Status: incident.LessonStatus(row.Status), CreatedAt: row.CreatedAt,
	}
	err := json.Unmarshal(row.Draft, &l.Draft)
	return l, err
}

func activeMemory(ctx context.Context, q *db.Queries, i incident.Incident, mode string) ([]incident.MemoryRule, error) {
	rows, err := q.ActiveLessons(ctx, db.ActiveLessonsParams{
		Mode: mode, MerchantID: i.Scope.MerchantID, PaymentMethod: i.Scope.Method, Environment: i.Scope.Environment,
		SourceWindowEnd: i.Scope.WindowEnd, ReplayAt: i.Clock, MaxRules: maxMemoryRules,
	})
	if err != nil {
		return nil, err
	}
	out := []incident.MemoryRule{}
	for _, row := range rows {
		l, err := decodeLesson(row)
		if err != nil {
			return nil, err
		}
		out = append(out, incident.MemoryRule{ID: l.ID, Condition: l.Draft.Condition, Check: l.Draft.Check, Rationale: l.Draft.Rationale})
	}
	return out, nil
}
