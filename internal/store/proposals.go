package store

import (
	"context"
	"fmt"
	"time"

	db "apm-investigator/internal/db/generated"
	"apm-investigator/internal/incident"
	"apm-investigator/internal/policy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func newProposal(i incident.Incident, draft policy.ProposalDraft) incident.Proposal {
	return incident.Proposal{
		ID:               uuid.New(),
		IncidentID:       i.ID,
		IncidentRevision: i.Revision,
		Type:             draft.Type,
		Owner:            draft.Owner,
		Title:            draft.Title,
		Body:             draft.Body,
		Status:           incident.ProposalPending,
		CreatedAt:        time.Now().UTC(),
	}
}
func insertProposal(ctx context.Context, q *db.Queries, i incident.Incident, p incident.Proposal) error {
	err := q.InsertProposal(ctx, db.InsertProposalParams{
		ID:               p.ID,
		IncidentID:       p.IncidentID,
		IncidentRevision: p.IncidentRevision,
		Type:             string(p.Type),
		Owner:            string(p.Owner),
		Title:            p.Title,
		Body:             p.Body,
		Status:           string(p.Status),
		CreatedAt:        p.CreatedAt,
	})
	if err != nil {
		return err
	}
	return appendTimeline(ctx, q, i, "proposal_created", p)
}
func proposal(r db.Proposal) incident.Proposal {
	p := incident.Proposal{
		ID:               r.ID,
		IncidentID:       r.IncidentID,
		IncidentRevision: r.IncidentRevision,
		Type:             incident.ProposalType(r.Type),
		Owner:            incident.Owner(r.Owner),
		Title:            r.Title,
		Body:             r.Body,
		Status:           incident.ProposalStatus(r.Status),
		CreatedAt:        r.CreatedAt,
		DecisionNote:     r.DecisionNote.String,
	}
	if r.DecidedAt.Valid {
		p.DecidedAt = &r.DecidedAt.Time
	}
	return p
}
func (s *Store) Proposal(ctx context.Context, id uuid.UUID) (incident.Proposal, error) {
	r, err := s.q.GetProposal(ctx, id)
	return proposal(r), err
}
func (s *Store) Proposals(ctx context.Context, id uuid.UUID) ([]incident.Proposal, error) {
	rows, err := s.q.ListProposals(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []incident.Proposal{}
	for _, r := range rows {
		out = append(out, proposal(r))
	}
	return out, nil
}
func (s *Store) Decide(ctx context.Context, id uuid.UUID, approve bool, note string) (incident.Proposal, error) {
	var p incident.Proposal
	err := s.tx(ctx, func(q *db.Queries) error {
		first, err := q.GetProposal(ctx, id)
		if err != nil {
			return err
		}
		i, err := decodeIncident(q.LockIncident(ctx, first.IncidentID))
		if err != nil {
			return err
		}
		row, err := q.GetProposal(ctx, id)
		if err != nil {
			return err
		}
		p = proposal(row)
		target := incident.ProposalRejected
		if approve {
			target = incident.ProposalApproved
		}
		if p.Status == target {
			return nil
		}
		if p.Status != incident.ProposalPending {
			return fmt.Errorf("%w: proposal already %s", incident.ErrInvalidInput, p.Status)
		}
		if p.IncidentRevision != i.Revision {
			return incident.ErrProposalStale
		}
		to, err := policy.AfterDecision(p.Type, approve, i.Impact)
		if err != nil {
			return err
		}
		if err = incident.Transition(i.Status, to); err != nil {
			return err
		}
		i.Status = to
		if err = update(ctx, q, &i, i.Revision); err != nil {
			return err
		}
		n, err := q.DecideProposal(ctx, db.DecideProposalParams{ID: id, Status: string(target), DecisionNote: pgtype.Text{String: note, Valid: true}})
		if err != nil {
			return err
		}
		if n != 1 {
			return incident.ErrProposalStale
		}
		if approve && p.Type == incident.ProposalEscalation {
			err = q.InsertTicket(ctx, db.InsertTicketParams{
				ID:         uuid.New(),
				ProposalID: id,
				IncidentID: i.ID,
				Owner:      string(p.Owner),
				Title:      p.Title,
				Body:       p.Body,
			})
			if err != nil {
				return err
			}
		}
		p.Status = target
		p.DecisionNote = note
		now := time.Now().UTC()
		p.DecidedAt = &now
		return appendTimeline(ctx, q, i, "proposal_"+string(target), p)
	})
	return p, err
}
func (s *Store) TicketCount(ctx context.Context, id uuid.UUID) (int64, error) {
	return s.q.CountTickets(ctx, id)
}
