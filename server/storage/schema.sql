CREATE TABLE IF NOT EXISTS cache (
  origin TEXT NOT NULL,
  invalidated INTEGER NOT NULL,
  lookup_time TEXT NOT NULL,
  expire_time TEXT NOT NULL,
  response_status_code INTEGER NOT NULL,
  response_headers TEXT,
  response_body TEXT,
  response_body_sha256 TEXT,
  error_message TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cache_origin ON cache (origin) WHERE invalidated = 0;
