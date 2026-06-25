-- Persist the GPS route polyline for a run as a JSON array of [lat, lng] pairs.
ALTER TABLE run_details ADD COLUMN route jsonb NOT NULL DEFAULT '[]'::jsonb;
