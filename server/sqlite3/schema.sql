CREATE TABLE IF NOT EXISTS cache (
  service_uri TEXT NOT NULL,
  generation TEXT NOT NULL,

  request_time TEXT NOT NULL,
  stale_time TEXT NOT NULL,

  error_code INTEGER,
  error_message TEXT,

  response_status_code INTEGER,
  response_headers TEXT,
  response_body_text TEXT,
  response_body_truncated INTEGER,

  invalidate_time TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cache_service_uri ON cache (service_uri) WHERE invalidate_time IS NULL;

CREATE INDEX IF NOT EXISTS idx_cache_generation ON cache (generation);
