-- 练习会话与答题记录

CREATE TABLE practice_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tool_code   VARCHAR(64) NOT NULL,
    mode        VARCHAR(32) NOT NULL,
    grade       VARCHAR(32) NOT NULL DEFAULT '',
    level       VARCHAR(32) NOT NULL DEFAULT '',
    word_set_id UUID REFERENCES word_sets(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_practice_sessions_user_id ON practice_sessions(user_id);
CREATE INDEX idx_practice_sessions_created_at ON practice_sessions(created_at DESC);

-- 单次练习中的每道题作答
CREATE TABLE practice_answers (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id     UUID NOT NULL REFERENCES practice_sessions(id) ON DELETE CASCADE,
    question_index INT NOT NULL,
    word_id        UUID NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    user_answer    TEXT NOT NULL DEFAULT '',
    correct_answer TEXT NOT NULL,
    is_correct     BOOLEAN NOT NULL,
    time_used_ms   BIGINT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_practice_answers_session_id ON practice_answers(session_id);
