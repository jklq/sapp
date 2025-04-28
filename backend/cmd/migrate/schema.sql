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
    spending_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, -- Date the spending actually occurred
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP, -- Date the record was created
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
    settled_at DATETIME DEFAULT NULL, -- Timestamp when this spending was included in a settlement, NULL if unsettled
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
    -- shared_mode TEXT NOT NULL, -- Removed: AI now infers apportionment from prompt
    shared_with INTEGER, -- User potentially sharing (references users.id), NULL if alone
    total_amount REAL NOT NULL,
    is_finished BOOLEAN DEFAULT 0,
    is_ambiguity_flagged BOOLEAN DEFAULT 0,
    ambiguity_flag_reason TEXT,
    error_message TEXT, -- Added: Store error messages if job fails
    pre_settled BOOLEAN DEFAULT 0, -- Added: Flag to indicate if the job's spendings should be settled immediately
    transaction_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, -- Date the transaction(s) in the prompt occurred
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

-- Transfers table logs when settlements occur between partners
CREATE TABLE IF NOT EXISTS transfers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    settled_by_user_id INTEGER NOT NULL, -- User initiating the settlement action in the app
    settled_with_user_id INTEGER NOT NULL, -- The other user involved in the settlement
    settlement_time DATETIME NOT NULL, -- Timestamp when the settlement was recorded
    FOREIGN KEY(settled_by_user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    FOREIGN KEY(settled_with_user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE
);

-- Partnerships table links two users together
CREATE TABLE IF NOT EXISTS partnerships (
    user1_id INTEGER NOT NULL,
    user2_id INTEGER NOT NULL,
    PRIMARY KEY (user1_id, user2_id), -- Ensures unique pairs, order matters due to CHECK
    FOREIGN KEY(user1_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY(user2_id) REFERENCES users(id) ON DELETE CASCADE,
    CHECK (user1_id < user2_id) -- Ensures consistent ordering (user1 always lower ID) and prevents self-partnership
);

-- Deposits table stores income/deposit information
CREATE TABLE IF NOT EXISTS deposits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL, -- User who received the deposit
    amount REAL NOT NULL,
    description TEXT,
    deposit_date DATETIME NOT NULL, -- Date the deposit was received/effective
    is_recurring BOOLEAN DEFAULT 0,
    recurrence_period TEXT, -- e.g., 'monthly', 'weekly', 'yearly', NULL if not recurring
    end_date DATETIME DEFAULT NULL, -- Date after which recurring deposit should stop generating occurrences, NULL if indefinite or not recurring
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE
);


-- Add settled_at column to user_spendings to track settlement status
-- We need to add this column separately as ALTER TABLE ADD COLUMN is standard SQL
-- Note: This migration script might need adjustment depending on how it's run.
-- If run multiple times, the ALTER TABLE might fail. Consider adding IF NOT EXISTS if supported.
-- For simplicity here, we assume it runs once or handles errors gracefully.
-- A more robust migration system would handle this better.

-- Seed default categories if they don't exist
INSERT OR IGNORE INTO categories (name, ai_notes) VALUES ('Groceries', 'dersom bruker nevner at en har kjøpt mat/middag uten videre forklaring, anta at dette er Groceries og ikke Eating Out');
INSERT OR IGNORE INTO categories (name) VALUES ('Transport');
INSERT OR IGNORE INTO categories (name) VALUES ('Eating Out');
INSERT OR IGNORE INTO categories (name) VALUES ('Entertainment (general)');
INSERT OR IGNORE INTO categories (name) VALUES ('Travel, Events & Vacation');
INSERT OR IGNORE INTO categories (name) VALUES ('Utilities');
INSERT OR IGNORE INTO categories (name, ai_notes) VALUES ('Technology', 'e.g. phone, computer, etc.');
INSERT OR IGNORE INTO categories (name) VALUES ('Subscription (general)');
INSERT OR IGNORE INTO categories (name) VALUES ('Coffee');
INSERT OR IGNORE INTO categories (name) VALUES ('Alcohol');
INSERT OR IGNORE INTO categories (name, ai_notes) VALUES ('Nutritional drink', 'e.g. Nutridrink, Fresubin, etc.');
INSERT OR IGNORE INTO categories (name) VALUES ('Rent/Mortgage');
INSERT OR IGNORE INTO categories (name) VALUES ('Shopping (general)');
INSERT OR IGNORE INTO categories (name) VALUES ('Clothes');
INSERT OR IGNORE INTO categories (name) VALUES ('Education');
INSERT OR IGNORE INTO categories (name) VALUES ('Health');
INSERT OR IGNORE INTO categories (name) VALUES ('Other');

INSERT OR IGNORE INTO categories (name) VALUES ('Energy Drinks');


-- Seed demo users (Password for demo_user is "password")
-- Use https://www.browserling.com/bcrypt or similar to generate hashes if needed.
-- Hash for "password": $2a$10$XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX (Replace X with actual hash)
-- Hash for "password" generated with cost 12:
INSERT OR IGNORE INTO users (id, username, password_hash, first_name) VALUES (1, 'demo_user', '$2a$12$JCHo4VpnfYXYxj7PQvhdFemKCwabcmK2NmtFXMK69b4rSoY5wHq8a', 'Demo');
INSERT OR IGNORE INTO users (id, username, password_hash, first_name) VALUES (2, 'partner_user', '$2a$12$JCHo4VpnfYXYxj7PQvhdFemKCwabcmK2NmtFXMK69b4rSoY5wHq8a', 'Partner'); -- Partner doesn't need to log in for this demo, use same hash for simplicity

-- Seed partnership for demo users (ensure user1_id < user2_id)
INSERT OR IGNORE INTO partnerships (user1_id, user2_id) VALUES (1, 2);
