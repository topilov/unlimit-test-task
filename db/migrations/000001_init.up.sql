CREATE TABLE incidents (
 id uuid PRIMARY KEY,
 status text NOT NULL CHECK(status IN ('investigating','awaiting_review','monitoring_recovery','closure_proposed','closed','manual_triage')),
 revision bigint NOT NULL CHECK(revision>0),
 merchant_id text NOT NULL, payment_method text NOT NULL, environment text NOT NULL,
 started_at timestamptz NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
 state jsonb NOT NULL
);
CREATE TABLE events (
 id uuid PRIMARY KEY, incident_id uuid NOT NULL REFERENCES incidents(id),
 source text NOT NULL, external_id text NOT NULL, kind text NOT NULL,
 occurred_at timestamptz NOT NULL, available_at timestamptz NOT NULL,
 payload jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(source,external_id), CHECK(available_at>=occurred_at)
);
CREATE TABLE investigation_runs (
 id uuid PRIMARY KEY, incident_id uuid NOT NULL REFERENCES incidents(id), base_revision bigint NOT NULL,
 mode text NOT NULL, status text NOT NULL, started_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz, result jsonb, error_message text NOT NULL DEFAULT '',
 turns jsonb NOT NULL DEFAULT '[]', UNIQUE(id,incident_id)
);
CREATE UNIQUE INDEX one_active_run ON investigation_runs(incident_id) WHERE status='running';
CREATE TABLE evidence (
 id uuid PRIMARY KEY, incident_id uuid NOT NULL REFERENCES incidents(id),
 run_id uuid NOT NULL, evidence_ref text NOT NULL, source text NOT NULL, kind text NOT NULL,
 observed_at timestamptz NOT NULL, available boolean NOT NULL, summary text NOT NULL,
 data jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(incident_id,evidence_ref), UNIQUE(run_id,evidence_ref),
 FOREIGN KEY(run_id,incident_id) REFERENCES investigation_runs(id,incident_id)
);
CREATE TABLE tool_runs (
 id uuid PRIMARY KEY, investigation_run_id uuid NOT NULL REFERENCES investigation_runs(id),
 step integer NOT NULL, tool_name text NOT NULL, arguments jsonb NOT NULL,
 status text NOT NULL, evidence_ref text, error_code text NOT NULL,
 started_at timestamptz NOT NULL, finished_at timestamptz NOT NULL,
 UNIQUE(investigation_run_id,step),
 FOREIGN KEY(investigation_run_id,evidence_ref) REFERENCES evidence(run_id,evidence_ref)
);
CREATE TABLE proposals (
 id uuid PRIMARY KEY, incident_id uuid NOT NULL REFERENCES incidents(id), incident_revision bigint NOT NULL,
 type text NOT NULL CHECK(type IN ('escalation','closure')),
 owner text NOT NULL, title text NOT NULL, body text NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','approved','rejected')),
 created_at timestamptz NOT NULL, decided_at timestamptz, decision_note text,
 UNIQUE(incident_id,incident_revision,type)
);
CREATE TABLE simulated_tickets (
 id uuid PRIMARY KEY, proposal_id uuid NOT NULL UNIQUE REFERENCES proposals(id),
 incident_id uuid NOT NULL REFERENCES incidents(id), owner text NOT NULL, title text NOT NULL,
 body text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE timeline (
 id bigserial PRIMARY KEY, incident_id uuid NOT NULL REFERENCES incidents(id),
 at timestamptz NOT NULL, kind text NOT NULL, data jsonb NOT NULL
);
CREATE INDEX timeline_incident ON timeline(incident_id,id);
