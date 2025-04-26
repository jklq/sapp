-- Users table stores user information
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    first_name TEXT -- Added first_name as it's used in categorization
);

-- Categories table stores spending categories
CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    ai_notes TEXT -- Added ai_notes used in prompt generation
    -- Removed unused mean/variation columns for now
);

-- Spendings table stores individual spending items, often created by AI categorization
CREATE TABLE IF NOT EXISTS spendings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    amount REAL NOT NULL,
    description TEXT,
    category INTEGER NOT NULL,
    made_by INTEGER NOT NULL, -- References the user who made the purchase
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(category) REFERENCES categories(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    FOREIGN KEY(made_by) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE
);

-- User_spendings links spendings to users and defines sharing details
CREATE TABLE IF NOT EXISTS user_spendings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    spending_id INTEGER NOT NULL,
    buyer INTEGER NOT NULL, -- User who paid (references users.id)
    shared_with INTEGER, -- User sharing the cost (references users.id), NULL if alone
    shared_user_takes_all BOOLEAN DEFAULT 0, -- True if shared_with user pays the full amount ('other' mode)
    FOREIGN KEY(spending_id) REFERENCES spendings(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(buyer) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(shared_with) REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL -- If shared user deleted, set to NULL
);

-- AI Categorization Jobs table tracks the status of AI categorization requests
CREATE TABLE IF NOT EXISTS ai_categorization_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    status TEXT NOT NULL DEFAULT 'queued', -- e.g., queued, processing, finished, failed
    prompt TEXT NOT NULL,
    buyer INTEGER NOT NULL, -- User who initiated the job (references users.id)
    shared_mode TEXT NOT NULL, -- 'alone', 'shared', 'mix' (from frontend)
    shared_with INTEGER, -- User potentially sharing (references users.id), NULL if alone
    total_amount REAL NOT NULL,
    is_finished BOOLEAN DEFAULT 0,
    is_ambiguity_flagged BOOLEAN DEFAULT 0,
    ambiguity_flag_reason TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status_updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(buyer) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(shared_with) REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL
);

-- AI Categorized Spendings links spendings created by AI back to the job
CREATE TABLE IF NOT EXISTS ai_categorized_spendings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    spending_id INTEGER NOT NULL UNIQUE, -- Each spending belongs to only one AI job
    job_id INTEGER NOT NULL,
    FOREIGN KEY(spending_id) REFERENCES spendings(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(job_id) REFERENCES ai_categorization_jobs(id) ON UPDATE CASCADE ON DELETE CASCADE
);

-- Seed default categories if they don't exist
INSERT OR IGNORE INTO categories (name) VALUES ('Groceries');
INSERT OR IGNORE INTO categories (name) VALUES ('Transport');
INSERT OR IGNORE INTO categories (name) VALUES ('Eating Out');
INSERT OR IGNORE INTO categories (name) VALUES ('Entertainment');
INSERT OR IGNORE INTO categories (name) VALUES ('Utilities');
INSERT OR IGNORE INTO categories (name) VALUES ('Rent/Mortgage');
INSERT OR IGNORE INTO categories (name) VALUES ('Shopping');
INSERT OR IGNORE INTO categories (name) VALUES ('Health');
INSERT OR IGNORE INTO categories (name) VALUES ('Other');

-- Seed demo users (Password for demo_user is "password")
-- Use https://www.browserling.com/bcrypt or similar to generate hashes if needed.
-- Hash for "password": $2a$10$XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX (Replace X with actual hash)
-- IMPORTANT: Replace the placeholder hash below with a real bcrypt hash for "password"
INSERT OR IGNORE INTO users (id, username, password_hash, first_name) VALUES (1, 'demo_user', '$2a$10$If1kDxkQLSTXp5hJzTVkjuvhXNlwszEc7zTHxQp/V3xNlUZJwZz0m', 'Demo');
INSERT OR IGNORE INTO users (id, username, password_hash, first_name) VALUES (2, 'partner_user', 'dummy_hash_partner', 'Partner'); -- Partner doesn't need to log in for this demo
