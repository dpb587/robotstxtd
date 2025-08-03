DROP TABLE IF EXISTS cache;

CREATE TABLE IF NOT EXISTS cache2 (
  service_uri TEXT NOT NULL,
  cache_key TEXT NOT NULL,

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

CREATE UNIQUE INDEX IF NOT EXISTS idx_cache2_service_uri ON cache2 (service_uri) WHERE invalidate_time IS NULL;
