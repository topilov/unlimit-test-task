-- name: GetRun :one
SELECT * FROM investigation_runs WHERE id=$1;

-- name: RunEvidence :many
SELECT data FROM evidence WHERE run_id=$1 ORDER BY evidence_ref;

-- name: InsertFeedback :one
INSERT INTO feedback(id,run_id,verdict,note)
SELECT sqlc.arg(id),r.id,sqlc.arg(verdict),sqlc.arg(note) FROM investigation_runs r WHERE r.id=sqlc.arg(run_id) AND r.status='completed'
RETURNING feedback.*;

-- name: GetFeedback :one
SELECT * FROM feedback WHERE id=$1;

-- name: CompleteReflection :execrows
UPDATE feedback SET reflection=$2, metadata=$3, error_message=''
WHERE id=$1 AND reflection IS NULL;

-- name: ReflectionFailed :exec
UPDATE feedback SET error_message=$2, metadata=$3 WHERE id=$1 AND reflection IS NULL;

-- name: InsertLesson :one
INSERT INTO lessons(id,feedback_id,source_run_id,mode,merchant_id,payment_method,environment,source_window_start,source_window_end,draft)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: GetLesson :one
SELECT * FROM lessons WHERE id=$1;

-- name: LessonForFeedback :one
SELECT * FROM lessons WHERE feedback_id=$1;

-- name: ListLessons :many
SELECT * FROM lessons ORDER BY created_at DESC,id;

-- name: ActivateLesson :execrows
UPDATE lessons SET status='active',activated_at=now() WHERE id=$1 AND status='pending';

-- name: DisableLesson :execrows
UPDATE lessons SET status='disabled',disabled_at=now() WHERE id=$1 AND status IN ('pending','active');

-- name: ActiveLessons :many
SELECT lessons.* FROM lessons
WHERE lessons.status='active' AND lessons.mode=sqlc.arg(mode) AND lessons.merchant_id=sqlc.arg(merchant_id)
 AND lessons.payment_method=sqlc.arg(payment_method) AND lessons.environment=sqlc.arg(environment)
 AND lessons.source_window_end <= sqlc.arg(source_window_end)
 AND EXISTS (
  SELECT 1 FROM investigation_runs r JOIN timeline t ON t.incident_id=r.incident_id
  WHERE r.id=lessons.source_run_id AND t.kind='investigation_completed'
   AND t.data->>'id'=r.id::text AND t.at <= sqlc.arg(replay_at)::timestamptz
 )
ORDER BY lessons.activated_at DESC,lessons.id LIMIT sqlc.arg(max_rules);
