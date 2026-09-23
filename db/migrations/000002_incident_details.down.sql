UPDATE incidents SET details = (details - 'window_end') || jsonb_build_object(
 'id',id,'status',status,'revision',revision,'created_at',created_at,'updated_at',updated_at,
 'scope',jsonb_build_object('merchant_id',merchant_id,'method',payment_method,
   'environment',environment,'window_start',started_at,'window_end',details->'window_end'));
ALTER TABLE incidents RENAME COLUMN details TO state;
