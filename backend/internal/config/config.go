package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppEnv                string
	Port                  string
	SessionSecret         string
	FrontendURL           string
	StravaClientID        string
	StravaClientSecret    string
	StravaRedirectURL     string
	StravaScopes          string
	SyncDefaultDays       int
	StaticDir             string
	AWSRegion             string
	DynamoAthletesTable   string
	DynamoActivitiesTable string
	DynamoActivitiesIndex string
	DynamoRoutesTable     string
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:                getenvDefault("APP_ENV", "development"),
		Port:                  getenvDefault("PORT", "8080"),
		SessionSecret:         os.Getenv("SESSION_SECRET"),
		FrontendURL:           getenvDefault("FRONTEND_URL", "http://localhost:5173"),
		StravaClientID:        os.Getenv("STRAVA_CLIENT_ID"),
		StravaClientSecret:    os.Getenv("STRAVA_CLIENT_SECRET"),
		StravaRedirectURL:     os.Getenv("STRAVA_REDIRECT_URL"),
		StravaScopes:          getenvDefault("STRAVA_SCOPES", "read,activity:read_all"),
		SyncDefaultDays:       getenvIntDefault("SYNC_DEFAULT_DAYS", 365),
		StaticDir:             os.Getenv("STATIC_DIR"),
		AWSRegion:             os.Getenv("AWS_REGION"),
		DynamoAthletesTable:   os.Getenv("DDB_ATHLETES_TABLE"),
		DynamoActivitiesTable: os.Getenv("DDB_ACTIVITIES_TABLE"),
		DynamoActivitiesIndex: getenvDefault("DDB_ACTIVITIES_BY_DATE_INDEX", "athlete_start_idx"),
		DynamoRoutesTable:     os.Getenv("DDB_ROUTES_TABLE"),
	}

	if cfg.StaticDir == "" {
		cfg.StaticDir = "../frontend/dist"
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) validate() error {
	var missing []string
	for key, value := range map[string]string{
		"SESSION_SECRET":               c.SessionSecret,
		"STRAVA_CLIENT_ID":             c.StravaClientID,
		"STRAVA_CLIENT_SECRET":         c.StravaClientSecret,
		"STRAVA_REDIRECT_URL":          c.StravaRedirectURL,
		"AWS_REGION":                   c.AWSRegion,
		"DDB_ATHLETES_TABLE":           c.DynamoAthletesTable,
		"DDB_ACTIVITIES_TABLE":         c.DynamoActivitiesTable,
		"DDB_ACTIVITIES_BY_DATE_INDEX": c.DynamoActivitiesIndex,
		"DDB_ROUTES_TABLE":             c.DynamoRoutesTable,
	} {
		if value == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %v", missing)
	}

	if len(c.SessionSecret) < 24 {
		return errors.New("SESSION_SECRET must be at least 24 characters")
	}

	if c.SyncDefaultDays <= 0 {
		return errors.New("SYNC_DEFAULT_DAYS must be greater than 0")
	}

	return nil
}

func getenvDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvIntDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
