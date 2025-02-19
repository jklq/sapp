CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
        username TEXT NOT NULL UNIQUE,
        password_hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
        mean_min_amount REAL,
        mean_max_amount REAL,
        variation REAL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		amount REAL NOT NULL,
        made_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        is_outstanding BOOLEAN DEFAULT 1,
        made_by INTEGER NOT NULL,
        shared_with INTEGER,
        category INTEGER,
        FOREIGN KEY(made_by) REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
        FOREIGN KEY(shared_with) REFERENCES users(id) ON UPDATE CASCADE,
        FOREIGN KEY(category) REFERENCES categories(id) ON UPDATE CASCADE
);