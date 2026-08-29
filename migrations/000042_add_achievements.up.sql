-- Gamification: badges catalogue, the badges users earned and the sign-up
-- date shown on the profile (member_since).

ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Badge catalogue. threshold is the value of metric (see internal/models:
-- BadgeMetric) at which the badge is earned; usecase.Achievements.Evaluate
-- compares the user's metrics with the catalogue, so a new badge is only a
-- new row here.
CREATE TABLE IF NOT EXISTS badges (
    code           TEXT PRIMARY KEY,
    name_ru        TEXT NOT NULL,
    name_en        TEXT NOT NULL,
    description_ru TEXT NOT NULL,
    description_en TEXT NOT NULL,
    icon           TEXT NOT NULL,
    threshold      INTEGER NOT NULL CHECK (threshold > 0),
    metric         TEXT NOT NULL
);

INSERT INTO badges (code, name_ru, name_en, description_ru, description_en, icon, threshold, metric) VALUES
    ('first_mark',   'Первая метка',  'First mark',   'Создана первая метка',                         'Created the first mark',                     'flag',    1,   'marks_total'),
    ('reporter_10',  'Репортёр',      'Reporter',     '10 подтверждённых меток',                      '10 confirmed marks',                         'megaphone', 10,  'marks_confirmed'),
    ('reporter_50',  'Корреспондент', 'Correspondent','50 подтверждённых меток',                      '50 confirmed marks',                         'newspaper', 50,  'marks_confirmed'),
    ('verifier_10',  'Проверяющий',   'Verifier',     '10 корректных проверок',                       '10 correct checks',                          'check',   10,  'checks_correct'),
    ('verifier_100', 'Инспектор',     'Inspector',    '100 корректных проверок',                      '100 correct checks',                         'shield',  100, 'checks_correct'),
    ('streak_7',     'Серия',         'Streak',       'Проверки 7 дней подряд',                       'Checks on 7 consecutive days',               'fire',    7,   'check_streak_days'),
    ('helper_5',     'Помощник',      'Helper',       '5 выполненных заданий',                        '5 completed tasks',                          'hands',   5,   'tasks_completed'),
    ('resolver',     'Решатель',      'Resolver',     'Первая метка доведена до статуса «Закрытая»',  'The first mark brought to the Closed status', 'trophy', 1,   'marks_closed')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS user_badges (
    user_id    INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    badge_code TEXT NOT NULL REFERENCES badges(code) ON DELETE CASCADE,
    earned_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, badge_code)
);

-- The leaderboard by boundary joins rating events to their marks.
CREATE INDEX IF NOT EXISTS idx_rating_events_mark_id ON rating_events (mark_id);
