ALTER TABLE investigation_runs ADD COLUMN memory jsonb NOT NULL DEFAULT '[]';

CREATE TABLE feedback (
 id uuid PRIMARY KEY,
 run_id uuid NOT NULL REFERENCES investigation_runs(id),
 verdict text NOT NULL CHECK(verdict IN ('helpful','incorrect','incomplete')),
 note text NOT NULL CHECK(length(trim(note)) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now(),
 reflection jsonb,
 metadata jsonb NOT NULL DEFAULT '{}',
 error_message text NOT NULL DEFAULT ''
);

CREATE TABLE lessons (
 id uuid PRIMARY KEY,
 feedback_id uuid NOT NULL UNIQUE REFERENCES feedback(id),
 source_run_id uuid NOT NULL REFERENCES investigation_runs(id),
 mode text NOT NULL CHECK(mode IN ('mock','live')),
 merchant_id text NOT NULL,
 payment_method text NOT NULL,
 environment text NOT NULL,
 source_window_start timestamptz NOT NULL,
 source_window_end timestamptz NOT NULL,
 draft jsonb NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','active','disabled')),
 created_at timestamptz NOT NULL DEFAULT now(),
 activated_at timestamptz,
 disabled_at timestamptz
);

CREATE INDEX active_lessons ON lessons(mode,merchant_id,payment_method,environment) WHERE status='active';
