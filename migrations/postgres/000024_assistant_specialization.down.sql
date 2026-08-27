ALTER TABLE vacancy_preferences
    DROP CONSTRAINT IF EXISTS vacancy_preferences_include_leadership_check,
    DROP CONSTRAINT IF EXISTS vacancy_preferences_specialization_check;
