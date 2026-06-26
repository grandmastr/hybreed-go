-- Unique on the lowercased name so the food catalogue can be re-seeded
-- idempotently (UpsertFood relies on this conflict target). USDA-sourced rows
-- carry no barcode, so the existing UNIQUE(barcode) is not enough.
CREATE UNIQUE INDEX IF NOT EXISTS idx_foods_lower_name ON foods (lower(name));
