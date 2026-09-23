ALTER TABLE incidents RENAME COLUMN state TO details;
UPDATE incidents SET details =
 (details - ARRAY['id','status','revision','scope','created_at','updated_at'])
 || jsonb_build_object('window_end', details->'scope'->'window_end');
