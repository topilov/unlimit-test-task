package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"apm-investigator/internal/incident"
)

type Payment struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Attempt struct {
	PaymentID  string    `json:"payment_id"`
	ID         string    `json:"id"`
	HTTPStatus int       `json:"http_status"`
	At         time.Time `json:"at"`
}
type Snapshot struct {
	Scope                incident.Scope `json:"scope"`
	AvailableAt          time.Time      `json:"available_at"`
	ObservedAt           time.Time      `json:"observed_at"`
	Available            bool           `json:"available"`
	Complete             bool           `json:"data_complete"`
	Payments             []Payment      `json:"payments"`
	Attempts             []Attempt      `json:"attempts"`
	QueueDepth           int            `json:"queue_depth"`
	OldestJobAge         int            `json:"oldest_job_age_seconds"`
	WorkerStatus         string         `json:"worker_status"`
	ProviderStatus       string         `json:"status"`
	PublishedAt          time.Time      `json:"published_at"`
	Text                 string         `json:"text"`
	NewTrafficChecked    int            `json:"new_traffic_checked"`
	NewTrafficSuccessful int            `json:"new_traffic_successful"`
}
type Sources struct{ Payments, Callbacks, Queue, Provider []Snapshot }

func Load(dir string) (*Sources, error) {
	s := &Sources{}
	for name, dst := range map[string]*[]Snapshot{
		"payments":  &s.Payments,
		"callbacks": &s.Callbacks,
		"queue":     &s.Queue,
		"provider":  &s.Provider,
	} {
		b, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, dst); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for _, v := range *dst {
			if v.AvailableAt.IsZero() || v.ObservedAt.IsZero() || v.AvailableAt.Before(v.ObservedAt) {
				return nil, fmt.Errorf("%s: invalid observation/availability timestamps", name)
			}
		}
	}
	return s, nil
}

func Latest(list []Snapshot, scope incident.Scope, now time.Time) (Snapshot, bool) {
	var result Snapshot
	found := false
	for _, s := range list {
		if !scope.SameIdentity(s.Scope) || s.AvailableAt.IsZero() || s.ObservedAt.IsZero() || s.AvailableAt.Before(s.ObservedAt) || s.AvailableAt.After(now) || s.ObservedAt.After(now) {
			continue
		}
		if !found || s.ObservedAt.After(result.ObservedAt) || (s.ObservedAt.Equal(result.ObservedAt) && s.AvailableAt.After(result.AvailableAt)) {
			result = s
			found = true
		} else if s.ObservedAt.Equal(result.ObservedAt) && s.AvailableAt.Equal(result.AvailableAt) {
			result.Complete = false
			result.Available = result.Available && s.Available
		}
	}
	return result, found && result.Available
}

const snapshotFreshness = 5 * time.Minute

func fresh(s Snapshot, now time.Time) bool {
	return s.Complete && !s.ObservedAt.IsZero() && now.Sub(s.ObservedAt) <= snapshotFreshness
}
func (s *Sources) PaymentSummary(scope incident.Scope, now time.Time) PaymentSummary {
	snap, ok := Latest(s.Payments, scope, now)
	out := PaymentSummary{DataComplete: ok && fresh(snap, now), CompletedIDs: []string{}, PaymentIDs: []string{}}
	if !ok {
		return out
	}
	byID := map[string]Payment{}
	conflicted := map[string]bool{}
	for _, p := range snap.Payments {
		if p.ID == "" || p.CreatedAt.IsZero() || p.UpdatedAt.Before(p.CreatedAt) {
			out.DataComplete = false
			continue
		}
		if p.UpdatedAt.After(snap.ObservedAt) || p.CreatedAt.Before(scope.WindowStart) || p.CreatedAt.After(scope.WindowEnd) {
			continue
		}
		old, exists := byID[p.ID]
		if !exists || p.UpdatedAt.After(old.UpdatedAt) {
			byID[p.ID] = p
			conflicted[p.ID] = false
		} else if p.UpdatedAt.Equal(old.UpdatedAt) && (p.Status != old.Status || !p.CreatedAt.Equal(old.CreatedAt)) {
			conflicted[p.ID] = true
		}
	}
	for id, p := range byID {
		if conflicted[id] {
			out.DataComplete = false
			continue
		}
		out.Total++
		out.PaymentIDs = append(out.PaymentIDs, id)
		switch p.Status {
		case "completed":
			out.Completed++
			out.CompletedIDs = append(out.CompletedIDs, id)
		case "pending":
			out.Pending++
		case "failed":
			out.Failed++
		default:
			out.Unknown++
			out.DataComplete = false
		}
	}
	sort.Strings(out.PaymentIDs)
	sort.Strings(out.CompletedIDs)
	return out
}
func (s *Sources) CallbackSummary(scope incident.Scope, now time.Time, ids []string) CallbackSummary {
	snap, ok := Latest(s.Callbacks, scope, now)
	out := CallbackSummary{
		PaymentsChecked: len(ids),
		DataComplete:    ok && fresh(snap, now),
		HTTPStatuses:    map[string]int{},
		Acknowledged:    []string{},
		SourceAvailable: ok,
	}
	if !ok {
		return out
	}
	allowed := map[string]bool{}
	for _, id := range ids {
		allowed[id] = true
	}
	attempts := map[string]bool{}
	success := map[string]bool{}
	seen := map[string]Attempt{}
	conflicts := map[string]bool{}
	for _, a := range snap.Attempts {
		// An unidentified record cannot safely be assigned outside this cohort.
		if strings.TrimSpace(a.PaymentID) == "" {
			out.DataComplete = false
			continue
		}
		if !allowed[a.PaymentID] {
			continue
		}
		if strings.TrimSpace(a.ID) == "" || a.At.IsZero() || a.At.After(snap.ObservedAt) || a.HTTPStatus < 100 || a.HTTPStatus > 599 {
			out.DataComplete = false
			continue
		}
		if old, exists := seen[a.ID]; exists {
			if old.PaymentID != a.PaymentID || old.HTTPStatus != a.HTTPStatus || !old.At.Equal(a.At) {
				out.DataComplete = false
				conflicts[a.ID] = true
			}
			continue
		}
		seen[a.ID] = a
	}
	for id, a := range seen {
		if conflicts[id] {
			continue
		}
		attempts[a.PaymentID] = true
		out.HTTPStatuses[fmt.Sprint(a.HTTPStatus)]++
		if a.HTTPStatus >= 200 && a.HTTPStatus < 300 {
			success[a.PaymentID] = true
		}
	}
	out.WithAttempts = len(attempts)
	out.Successful = len(success)
	for id := range success {
		out.Acknowledged = append(out.Acknowledged, id)
	}
	sort.Strings(out.Acknowledged)
	out.NewTrafficChecked = snap.NewTrafficChecked
	out.NewTrafficSuccessful = snap.NewTrafficSuccessful
	if out.NewTrafficChecked < 0 || out.NewTrafficSuccessful < 0 || out.NewTrafficSuccessful > out.NewTrafficChecked {
		out.DataComplete = false
	}
	return out
}

type PaymentSummary struct {
	Total        int      `json:"total_unique_payments"`
	Completed    int      `json:"completed"`
	Pending      int      `json:"pending"`
	Failed       int      `json:"failed"`
	Unknown      int      `json:"unknown"`
	PaymentIDs   []string `json:"payment_ids"`
	CompletedIDs []string `json:"completed_payment_ids"`
	DataComplete bool     `json:"data_complete"`
}
type CallbackSummary struct {
	PaymentsChecked      int            `json:"payments_checked"`
	WithAttempts         int            `json:"with_attempts"`
	Successful           int            `json:"with_successful_delivery"`
	HTTPStatuses         map[string]int `json:"http_status_counts"`
	Acknowledged         []string       `json:"acknowledged_payment_ids"`
	DataComplete         bool           `json:"data_complete"`
	SourceAvailable      bool           `json:"source_available"`
	NewTrafficChecked    int            `json:"new_traffic_checked"`
	NewTrafficSuccessful int            `json:"new_traffic_successful"`
}

func (s *Sources) Base(scope incident.Scope, now time.Time, critical bool) (incident.Impact, []string) {
	p := s.PaymentSummary(scope, now)
	c := s.CallbackSummary(scope, now, p.CompletedIDs)
	acked := map[string]bool{}
	for _, id := range c.Acknowledged {
		acked[id] = true
	}
	cohort := []string{}
	for _, id := range p.CompletedIDs {
		if !acked[id] {
			cohort = append(cohort, id)
		}
	}
	return incident.Impact{
		CohortComplete:   p.DataComplete,
		UniquePayments:   p.Total,
		AffectedPayments: len(cohort),
		CustomerVisible:  len(cohort) > 0,
		CriticalMerchant: critical,
		WindowStart:      scope.WindowStart,
		WindowEnd:        scope.WindowEnd,
	}, cohort
}
