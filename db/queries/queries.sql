-- name: CreateIncident :exec
INSERT INTO incidents(id,status,revision,merchant_id,payment_method,environment,started_at,created_at,updated_at,details) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);
-- name: GetIncident :one
SELECT * FROM incidents WHERE id=$1;
-- name: LockIncident :one
SELECT * FROM incidents WHERE id=$1 FOR UPDATE;
-- name: ListIncidents :many
SELECT * FROM incidents ORDER BY created_at DESC,id;
-- name: UpdateIncident :execrows
UPDATE incidents SET status=sqlc.arg(status), revision=revision+1,
 details=jsonb_set(sqlc.arg(details)::jsonb,'{cursor}',
   to_jsonb(GREATEST((details->>'cursor')::integer,(sqlc.arg(details)::jsonb->>'cursor')::integer))),
 updated_at=sqlc.arg(updated_at)
WHERE id=sqlc.arg(id) AND revision=sqlc.arg(revision);
-- name: InsertEvent :execrows
INSERT INTO events(id,incident_id,source,external_id,kind,occurred_at,available_at,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(source,external_id) DO NOTHING;
-- name: ListEvents :many
SELECT * FROM events WHERE incident_id=$1 ORDER BY available_at,id;
-- name: StartRun :exec
INSERT INTO investigation_runs(id,incident_id,base_revision,mode,status,memory,context) VALUES($1,$2,$3,$4,'running',$5,$6);
-- name: UpdateRun :exec
UPDATE investigation_runs SET status=$2,result=$3,error_message=$4,turns=$5,finished_at=CASE WHEN $2='running' THEN NULL ELSE now() END WHERE id=$1 AND status='running';
-- name: ListRuns :many
SELECT * FROM investigation_runs WHERE incident_id=$1 ORDER BY started_at,id;
-- name: InsertEvidence :exec
INSERT INTO evidence(id,incident_id,run_id,evidence_ref,source,kind,observed_at,available,summary,data) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);
-- name: ListEvidence :many
SELECT data FROM evidence WHERE incident_id=$1 ORDER BY created_at,evidence_ref;
-- name: InsertToolRun :exec
INSERT INTO tool_runs(id,investigation_run_id,step,tool_name,arguments,status,evidence_ref,error_code,started_at,finished_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);
-- name: ListToolRuns :many
SELECT * FROM tool_runs WHERE investigation_run_id=$1 ORDER BY step;
-- name: InsertProposal :exec
INSERT INTO proposals(id,incident_id,incident_revision,type,owner,title,body,status,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);
-- name: GetProposal :one
SELECT * FROM proposals WHERE id=$1;
-- name: ListProposals :many
SELECT * FROM proposals WHERE incident_id=$1 ORDER BY created_at,id;
-- name: DecideProposal :execrows
UPDATE proposals SET status=$2,decided_at=now(),decision_note=$3 WHERE id=$1 AND status='pending';
-- name: InsertTicket :exec
INSERT INTO simulated_tickets(id,proposal_id,incident_id,owner,title,body) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(proposal_id) DO NOTHING;
-- name: CountTickets :one
SELECT count(*) FROM simulated_tickets WHERE proposal_id=$1;
-- name: AppendTimeline :exec
INSERT INTO timeline(incident_id,at,kind,data) VALUES($1,$2,$3,$4);
-- name: Timeline :many
SELECT at,kind,data FROM timeline WHERE incident_id=$1 ORDER BY id;

-- name: AdvanceReplayCursor :exec
UPDATE incidents SET details=jsonb_set(details,'{cursor}',to_jsonb(sqlc.arg(cursor)::integer)) WHERE id=sqlc.arg(id);
