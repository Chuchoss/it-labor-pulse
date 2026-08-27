-- Repair is_remote from retained official HH detail fields. Search-only
-- touches must not keep a later NULL over a known schedule/work_format fact.
UPDATE vacancies
SET is_remote = CASE
    WHEN lower(COALESCE(raw_payload -> 'schedule' ->> 'id', '')) IN ('remote', 'remote_work')
      OR EXISTS (
          SELECT 1
          FROM jsonb_array_elements(
              CASE
                  WHEN jsonb_typeof(raw_payload -> 'work_format') = 'array'
                      THEN raw_payload -> 'work_format'
                  ELSE '[]'::jsonb
              END
          ) AS wf
          WHERE upper(COALESCE(wf ->> 'id', '')) IN ('REMOTE', 'REMOTE_WORK')
      )
        THEN true
    WHEN NULLIF(raw_payload -> 'schedule' ->> 'id', '') IS NOT NULL
      OR (
          jsonb_typeof(raw_payload -> 'work_format') = 'array'
          AND jsonb_array_length(raw_payload -> 'work_format') > 0
      )
        THEN false
    ELSE is_remote
END
WHERE source = 'hh'
  AND raw_payload IS NOT NULL
  AND is_remote IS DISTINCT FROM (
      CASE
          WHEN lower(COALESCE(raw_payload -> 'schedule' ->> 'id', '')) IN ('remote', 'remote_work')
            OR EXISTS (
                SELECT 1
                FROM jsonb_array_elements(
                    CASE
                        WHEN jsonb_typeof(raw_payload -> 'work_format') = 'array'
                            THEN raw_payload -> 'work_format'
                        ELSE '[]'::jsonb
                    END
                ) AS wf
                WHERE upper(COALESCE(wf ->> 'id', '')) IN ('REMOTE', 'REMOTE_WORK')
            )
              THEN true
          WHEN NULLIF(raw_payload -> 'schedule' ->> 'id', '') IS NOT NULL
            OR (
                jsonb_typeof(raw_payload -> 'work_format') = 'array'
                AND jsonb_array_length(raw_payload -> 'work_format') > 0
            )
              THEN false
          ELSE is_remote
      END
  );
