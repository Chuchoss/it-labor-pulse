ALTER TABLE vacancy_preferences
    ADD CONSTRAINT vacancy_preferences_specialization_check
    CHECK (
        NOT (hard_criteria ? 'specialization')
        OR (
            jsonb_typeof(hard_criteria->'specialization') = 'string'
            AND hard_criteria->>'specialization' IN (
                'frontend', 'backend', 'fullstack', 'mobile',
                'devops_platform', 'data_ml', 'other'
            )
        )
    ),
    ADD CONSTRAINT vacancy_preferences_include_leadership_check
    CHECK (
        NOT (hard_criteria ? 'include_leadership')
        OR jsonb_typeof(hard_criteria->'include_leadership') = 'boolean'
    );

COMMENT ON CONSTRAINT vacancy_preferences_specialization_check ON vacancy_preferences IS
    'Canonical developer specialization in an immutable preference version.';
COMMENT ON CONSTRAINT vacancy_preferences_include_leadership_check ON vacancy_preferences IS
    'Explicit opt-in for lead, team lead and head vacancies; absent means false.';
