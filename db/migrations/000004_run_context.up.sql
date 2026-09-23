ALTER TABLE investigation_runs ADD COLUMN context jsonb NOT NULL DEFAULT '{}';
