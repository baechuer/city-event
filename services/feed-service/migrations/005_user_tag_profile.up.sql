-- user_tag_profile with VIEW swap pattern
-- Two backing tables + view for atomic switch

CREATE TABLE user_tag_profile_a (
    actor_key TEXT NOT NULL,
    tag TEXT NOT NULL,
    weight FLOAT DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (actor_key, tag)
);

CREATE TABLE user_tag_profile_b (
    actor_key TEXT NOT NULL,
    tag TEXT NOT NULL,
    weight FLOAT DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (actor_key, tag)
);

-- Active profile view (points to _a initially)
CREATE VIEW user_tag_profile AS SELECT * FROM user_tag_profile_a;

-- Track which table is active
CREATE TABLE user_tag_profile_config (
    id INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    active_table TEXT NOT NULL DEFAULT 'a' CHECK (active_table IN ('a', 'b'))
);

INSERT INTO user_tag_profile_config (id, active_table) VALUES (1, 'a');
