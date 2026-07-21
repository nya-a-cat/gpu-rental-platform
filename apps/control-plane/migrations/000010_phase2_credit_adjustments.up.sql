ALTER TABLE ledger_entries ALTER COLUMN usage_fact_id DROP NOT NULL;
ALTER TABLE ledger_entries ADD COLUMN description text NOT NULL DEFAULT '';
