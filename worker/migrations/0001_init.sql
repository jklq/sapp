-- Same schema as the Go backend's schema.sql. Timestamps are stored as ISO 8601
-- UTC strings ("2026-03-24T09:18:20.659Z") so they compare correctly as text.

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    first_name TEXT
);

CREATE TABLE categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    ai_notes TEXT
);

CREATE TABLE spendings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    amount REAL NOT NULL,
    description TEXT,
    category INTEGER NOT NULL,
    made_by INTEGER NOT NULL,
    spending_date DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY(category) REFERENCES categories(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    FOREIGN KEY(made_by) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE TABLE user_spendings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    spending_id INTEGER NOT NULL,
    buyer INTEGER NOT NULL,
    shared_with INTEGER,
    shared_user_takes_all BOOLEAN DEFAULT 0,
    settled_at DATETIME DEFAULT NULL,
    FOREIGN KEY(spending_id) REFERENCES spendings(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(buyer) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(shared_with) REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL
);

CREATE TABLE ai_categorization_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    status TEXT NOT NULL DEFAULT 'queued',
    prompt TEXT NOT NULL,
    buyer INTEGER NOT NULL,
    shared_with INTEGER,
    total_amount REAL NOT NULL,
    is_finished BOOLEAN DEFAULT 0,
    is_ambiguity_flagged BOOLEAN DEFAULT 0,
    ambiguity_flag_reason TEXT,
    error_message TEXT,
    pre_settled BOOLEAN DEFAULT 0,
    transaction_date DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    status_updated_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY(buyer) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(shared_with) REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL
);

CREATE TABLE ai_categorized_spendings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    spending_id INTEGER NOT NULL UNIQUE,
    job_id INTEGER NOT NULL,
    FOREIGN KEY(spending_id) REFERENCES spendings(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(job_id) REFERENCES ai_categorization_jobs(id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE TABLE transfers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    settled_by_user_id INTEGER NOT NULL,
    settled_with_user_id INTEGER NOT NULL,
    settlement_time DATETIME NOT NULL,
    FOREIGN KEY(settled_by_user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(settled_with_user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE TABLE partnerships (
    user1_id INTEGER NOT NULL,
    user2_id INTEGER NOT NULL,
    PRIMARY KEY (user1_id, user2_id),
    FOREIGN KEY(user1_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY(user2_id) REFERENCES users(id) ON DELETE CASCADE,
    CHECK (user1_id < user2_id)
);

CREATE TABLE deposits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    amount REAL NOT NULL,
    description TEXT,
    deposit_date DATETIME NOT NULL,
    is_recurring BOOLEAN DEFAULT 0,
    recurrence_period TEXT,
    end_date DATETIME DEFAULT NULL,
    created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY(user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE TABLE refresh_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);

INSERT INTO categories (name, ai_notes) VALUES
    ('Groceries', 'dersom bruker nevner at en har kjøpt mat/middag uten videre forklaring, anta at dette er Groceries og ikke Eating Out'),
    ('Eating Out', 'mat og drikke på restaurant, cafe, bar, osv. inkluderer take away. inkluderer ikke kaffe, se seperat kategori.'),
    ('Coffee', NULL),
    ('Entertainment (general)', NULL),
    ('Travel, Events & Vacation', NULL),
    ('Transport', NULL),
    ('Utilities', NULL),
    ('Technology', 'e.g. phone, computer, osv.'),
    ('Subscription (general)', NULL),
    ('Alcohol', NULL),
    ('Nutritional drink', 'e.g. Nutridrink, Fresubin, osv.'),
    ('Rent/Mortgage', NULL),
    ('Shopping (general)', NULL),
    ('Clothes', NULL),
    ('Education', NULL),
    ('Health', NULL),
    ('Other', NULL),
    ('Energy Drinks', NULL),
    ('Digital Services', NULL);
