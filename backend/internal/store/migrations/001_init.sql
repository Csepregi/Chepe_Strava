CREATE TABLE IF NOT EXISTS athletes (
  id BIGINT PRIMARY KEY,
  username TEXT,
  firstname TEXT,
  lastname TEXT,
  city TEXT,
  state TEXT,
  country TEXT,
  profile_medium TEXT,
  profile TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS oauth_tokens (
  athlete_id BIGINT PRIMARY KEY REFERENCES athletes(id) ON DELETE CASCADE,
  access_token TEXT NOT NULL,
  refresh_token TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  scope TEXT,
  token_type TEXT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS activities (
  id BIGINT PRIMARY KEY,
  athlete_id BIGINT NOT NULL REFERENCES athletes(id) ON DELETE CASCADE,
  name TEXT,
  type TEXT,
  distance_meters DOUBLE PRECISION NOT NULL DEFAULT 0,
  moving_time_seconds INTEGER NOT NULL DEFAULT 0,
  elapsed_time_seconds INTEGER NOT NULL DEFAULT 0,
  total_elevation_gain DOUBLE PRECISION NOT NULL DEFAULT 0,
  average_speed DOUBLE PRECISION NOT NULL DEFAULT 0,
  max_speed DOUBLE PRECISION NOT NULL DEFAULT 0,
  start_date TIMESTAMPTZ,
  timezone TEXT,
  raw JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_activities_athlete_start
  ON activities (athlete_id, start_date DESC);
