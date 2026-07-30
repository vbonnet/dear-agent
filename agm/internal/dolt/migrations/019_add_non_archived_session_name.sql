-- AGM Migration 019: Non-Archived Session Name Projection
-- Archived rows project NULL so their historical names remain reusable.

ALTER TABLE agm_sessions
  ADD COLUMN non_archived_name VARCHAR(255)
    GENERATED ALWAYS AS (
      CASE
        WHEN status != 'archived' AND name IS NOT NULL AND name != ''
          THEN name
        ELSE NULL
      END
    ) STORED;
