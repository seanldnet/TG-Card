CREATE TABLE IF NOT EXISTS site_settings (
 id INTEGER PRIMARY KEY,
 settings_json TEXT NOT NULL DEFAULT '{}',
 updated_at TEXT NOT NULL
);
