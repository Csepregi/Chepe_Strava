package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"stravaapp/backend/internal/config"
	"stravaapp/backend/internal/store"
	"stravaapp/backend/internal/strava"
)

type App struct {
	cfg          config.Config
	store        *store.Store
	strava       *strava.Client
	secureCookie bool
	staticDir    string
}

type syncRequest struct {
	AfterDays int `json:"after_days"`
}

type syncResponse struct {
	FetchedActivities int `json:"fetched_activities"`
	PagesFetched      int `json:"pages_fetched"`
	AfterDays         int `json:"after_days"`
}

type routeSyncResponse struct {
	SyncedRoutes  int `json:"synced_routes"`
	SkippedRoutes int `json:"skipped_routes"`
}

type meResponse struct {
	Athlete        store.Athlete `json:"athlete"`
	TokenExpiresAt time.Time     `json:"token_expires_at"`
}

type visualPoint struct {
	DistanceMeters float64  `json:"distance_meters"`
	AltitudeMeters float64  `json:"altitude_meters"`
	Lat            *float64 `json:"lat,omitempty"`
	Lng            *float64 `json:"lng,omitempty"`
}

type activityVisualResponse struct {
	Description              string        `json:"description"`
	PhotoCount               int           `json:"photo_count"`
	PrimaryPhotoURL          string        `json:"primary_photo_url"`
	PrimaryPhotoThumbnailURL string        `json:"primary_photo_thumbnail_url"`
	StreamPoints             []visualPoint `json:"stream_points"`
}

type routeVisualResponse struct {
	StreamPoints []visualPoint `json:"stream_points"`
}

type analyticsTrendBucketResponse struct {
	Label                   string   `json:"label"`
	Start                   string   `json:"start"`
	End                     string   `json:"end"`
	ActivityCount           int      `json:"activity_count"`
	DistanceMeters          float64  `json:"distance_meters"`
	MovingTimeSeconds       int      `json:"moving_time_seconds"`
	ElevationGain           float64  `json:"elevation_gain"`
	AveragePaceSecondsPerKm *float64 `json:"average_pace_seconds_per_km,omitempty"`
}

type analyticsRecordsResponse struct {
	BestPaceSecondsPerKm *float64 `json:"best_pace_seconds_per_km,omitempty"`
	MostElevationGain    float64  `json:"most_elevation_gain"`
	HighestMaxSpeed      float64  `json:"highest_max_speed"`
	LongestDuration      int      `json:"longest_duration_seconds"`
}

type analyticsTrendsResponse struct {
	Weekly  []analyticsTrendBucketResponse `json:"weekly"`
	Monthly []analyticsTrendBucketResponse `json:"monthly"`
	Records analyticsRecordsResponse       `json:"records"`
}

func New(cfg config.Config, dataStore *store.Store) *App {
	staticDir := ""
	if info, err := os.Stat(cfg.StaticDir); err == nil && info.IsDir() {
		staticDir = cfg.StaticDir
	}

	return &App{
		cfg:          cfg,
		store:        dataStore,
		strava:       strava.NewClient(cfg.StravaClientID, cfg.StravaClientSecret, cfg.StravaRedirectURL, cfg.StravaScopes),
		secureCookie: cfg.AppEnv == "production",
		staticDir:    staticDir,
	}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("GET /api/auth/strava/login", a.handleStravaLogin)
	mux.HandleFunc("GET /api/auth/strava/callback", a.handleStravaCallback)
	mux.HandleFunc("POST /api/auth/logout", a.requireAuth(a.handleLogout))
	mux.HandleFunc("GET /api/public/latest-activity", a.handleLatestPublicActivity)
	mux.HandleFunc("GET /api/search", a.handleSearch)
	mux.HandleFunc("GET /api/activities/{activityID}/visual", a.handleActivityVisual)
	mux.HandleFunc("GET /api/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("POST /api/activities/sync", a.requireAuth(a.handleSyncActivities))
	mux.HandleFunc("GET /api/activities", a.requireAuth(a.handleListActivities))
	mux.HandleFunc("GET /api/analytics/summary", a.requireAuth(a.handleSummary))
	mux.HandleFunc("GET /api/analytics/trends", a.requireAuth(a.handleTrends))
	mux.HandleFunc("POST /api/routes/sync", a.requireAuth(a.handleSyncRoutes))
	mux.HandleFunc("GET /api/routes/search", a.handleRouteSearch)
	mux.HandleFunc("GET /api/routes/{routeID}/visual", a.handleRouteVisual)
	mux.HandleFunc("GET /api/routes/{routeID}", a.handleRouteDetail)

	if a.staticDir != "" {
		mux.HandleFunc("/", a.handleSPA)
	}

	return a.loggingMiddleware(mux)
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleStravaLogin(w http.ResponseWriter, r *http.Request) {
	stateCookie, state, err := createStateCookie(a.secureCookie)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not initialize oauth state")
		return
	}

	http.SetCookie(w, stateCookie)
	http.Redirect(w, r, a.strava.AuthURL(state), http.StatusTemporaryRedirect)
}

func (a *App) handleStravaCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing oauth code/state")
		return
	}

	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing oauth state cookie")
		return
	}
	http.SetCookie(w, clearStateCookie(a.secureCookie))

	if subtle.ConstantTimeCompare([]byte(state), []byte(cookie.Value)) != 1 {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}

	tokenData, err := a.strava.ExchangeCode(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusBadGateway, "oauth token exchange failed")
		return
	}

	athlete := store.Athlete{
		ID:            tokenData.Athlete.ID,
		Username:      tokenData.Athlete.Username,
		FirstName:     tokenData.Athlete.Firstname,
		LastName:      tokenData.Athlete.Lastname,
		City:          tokenData.Athlete.City,
		State:         tokenData.Athlete.State,
		Country:       tokenData.Athlete.Country,
		ProfileMedium: tokenData.Athlete.ProfileMedium,
		Profile:       tokenData.Athlete.Profile,
	}
	storedToken := store.Token{
		AthleteID:    athlete.ID,
		AccessToken:  tokenData.AccessToken,
		RefreshToken: tokenData.RefreshToken,
		ExpiresAt:    time.Unix(tokenData.ExpiresAt, 0).UTC(),
		Scope:        a.cfg.StravaScopes,
		TokenType:    tokenData.TokenType,
	}

	if err := a.store.UpsertAthleteAndToken(r.Context(), athlete, storedToken); err != nil {
		log.Printf("failed to persist athlete session for athlete_id=%d: %v", athlete.ID, err)
		writeError(w, http.StatusInternalServerError, "could not persist athlete session")
		return
	}

	sessionCookie, err := makeSessionCookie(a.cfg.SessionSecret, athlete.ID, 30*24*time.Hour, a.secureCookie)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	http.SetCookie(w, sessionCookie)

	target := strings.TrimRight(a.cfg.FrontendURL, "/") + "/"
	http.Redirect(w, r, target, http.StatusTemporaryRedirect)
}

func (a *App) handleLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, clearSessionCookie(a.secureCookie))
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	athleteID, ok := athleteIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	athlete, err := a.store.GetAthlete(r.Context(), athleteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unknown athlete")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch athlete")
		return
	}

	token, err := a.store.GetToken(r.Context(), athleteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "missing token")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch token")
		return
	}

	writeJSON(w, http.StatusOK, meResponse{Athlete: athlete, TokenExpiresAt: token.ExpiresAt})
}

func (a *App) handleLatestPublicActivity(w http.ResponseWriter, r *http.Request) {
	activity, err := a.store.LatestActivity(r.Context())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"activity": nil})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch latest activity")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"activity": activity})
}

func (a *App) handleSyncActivities(w http.ResponseWriter, r *http.Request) {
	athleteID, ok := athleteIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	reqBody := syncRequest{AfterDays: a.cfg.SyncDefaultDays}
	if r.Body != nil {
		defer r.Body.Close()
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decodeErr := decoder.Decode(&reqBody); decodeErr != nil && !errors.Is(decodeErr, io.EOF) && !errors.Is(decodeErr, context.Canceled) {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if reqBody.AfterDays <= 0 {
		reqBody.AfterDays = a.cfg.SyncDefaultDays
	}

	token, err := a.store.GetToken(r.Context(), athleteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "missing oauth token")
		return
	}

	accessToken, err := a.ensureValidToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to refresh strava token")
		return
	}

	after := time.Now().AddDate(0, 0, -reqBody.AfterDays)
	const perPage = 200
	const maxPages = 50

	totalFetched := 0
	pagesFetched := 0

	for page := 1; page <= maxPages; page++ {
		activities, fetchErr := a.strava.FetchActivities(r.Context(), accessToken, perPage, page, after)
		if fetchErr != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("failed fetching activities page %d", page))
			return
		}

		pagesFetched++
		if len(activities) == 0 {
			break
		}

		mapped, mapErr := mapActivities(athleteID, activities)
		if mapErr != nil {
			writeError(w, http.StatusInternalServerError, "failed processing activities")
			return
		}
		if upsertErr := a.store.UpsertActivities(r.Context(), athleteID, mapped); upsertErr != nil {
			writeError(w, http.StatusInternalServerError, "failed storing activities")
			return
		}

		totalFetched += len(mapped)
		if len(activities) < perPage {
			break
		}
	}

	writeJSON(w, http.StatusOK, syncResponse{
		FetchedActivities: totalFetched,
		PagesFetched:      pagesFetched,
		AfterDays:         reqBody.AfterDays,
	})
}

func (a *App) handleListActivities(w http.ResponseWriter, r *http.Request) {
	athleteID, ok := athleteIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = parsed
	}

	activities, err := a.store.ListActivities(r.Context(), athleteID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list activities")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"activities": activities})
}

func (a *App) handleSummary(w http.ResponseWriter, r *http.Request) {
	athleteID, ok := athleteIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	days := 90
	if rawDays := r.URL.Query().Get("days"); rawDays != "" {
		parsed, err := strconv.Atoi(rawDays)
		if err != nil || parsed <= 0 || parsed > 3650 {
			writeError(w, http.StatusBadRequest, "days must be between 1 and 3650")
			return
		}
		days = parsed
	}

	summary, err := a.store.Summary(r.Context(), athleteID, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load summary")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"days": days, "summary": summary})
}

func (a *App) handleTrends(w http.ResponseWriter, r *http.Request) {
	athleteID, ok := athleteIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	activities, err := a.store.ListAllActivities(r.Context(), athleteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load trend data")
		return
	}

	payload := analyticsTrendsResponse{
		Weekly:  buildWeeklyTrendBuckets(activities, 10, time.Now().UTC()),
		Monthly: buildMonthlyTrendBuckets(activities, 12, time.Now().UTC()),
		Records: buildAnalyticsRecords(activities),
	}

	writeJSON(w, http.StatusOK, payload)
}

func (a *App) handleSyncRoutes(w http.ResponseWriter, r *http.Request) {
	athleteID, ok := athleteIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	token, err := a.store.GetToken(r.Context(), athleteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "missing oauth token")
		return
	}

	accessToken, err := a.ensureValidToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to refresh strava token")
		return
	}

	routes, err := a.strava.ListAthleteRoutes(r.Context(), accessToken, athleteID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch routes")
		return
	}

	mappedRoutes := make([]store.Route, 0, len(routes))
	skippedRoutes := 0
	for _, summaryRoute := range routes {
		detailedRoute, routeErr := a.strava.GetRouteByID(r.Context(), accessToken, summaryRoute.ID)
		if routeErr != nil {
			log.Printf("failed to fetch route detail for route_id=%d: %v", summaryRoute.ID, routeErr)
			skippedRoutes++
			continue
		}

		mappedRoute, mapErr := mapRoute(athleteID, detailedRoute)
		if mapErr != nil {
			writeError(w, http.StatusInternalServerError, "failed processing routes")
			return
		}
		mappedRoutes = append(mappedRoutes, mappedRoute)
	}

	if err := a.store.UpsertRoutes(r.Context(), mappedRoutes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed storing routes")
		return
	}

	writeJSON(w, http.StatusOK, routeSyncResponse{
		SyncedRoutes:  len(mappedRoutes),
		SkippedRoutes: skippedRoutes,
	})
}

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	limit := 12
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 || parsed > 50 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 50")
			return
		}
		limit = parsed
	}

	query := r.URL.Query().Get("q")
	activities, err := a.store.SearchActivities(r.Context(), query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search activities")
		return
	}

	routes, err := a.store.SearchRoutes(r.Context(), query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search routes")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"query":      query,
		"activities": activities,
		"routes":     routes,
	})
}

func (a *App) handleActivityVisual(w http.ResponseWriter, r *http.Request) {
	activityID, err := strconv.ParseInt(r.PathValue("activityID"), 10, 64)
	if err != nil || activityID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid activity id")
		return
	}

	athleteID, err := strconv.ParseInt(r.URL.Query().Get("athlete_id"), 10, 64)
	if err != nil || athleteID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid athlete id")
		return
	}

	if _, err := a.store.GetActivity(r.Context(), athleteID, activityID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "activity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load activity")
		return
	}

	token, err := a.store.GetToken(r.Context(), athleteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "missing oauth token")
		return
	}

	accessToken, err := a.ensureValidToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to refresh strava token")
		return
	}

	detail, err := a.strava.GetActivityByID(r.Context(), accessToken, activityID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch activity detail")
		return
	}

	streams, err := a.strava.GetActivityStreams(r.Context(), accessToken, activityID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch activity streams")
		return
	}

	writeJSON(w, http.StatusOK, activityVisualResponse{
		Description:              detail.Description,
		PhotoCount:               detail.Photos.Count,
		PrimaryPhotoURL:          bestPhotoURL(detail),
		PrimaryPhotoThumbnailURL: thumbnailPhotoURL(detail),
		StreamPoints:             buildVisualPoints(streams),
	})
}

func (a *App) handleRouteSearch(w http.ResponseWriter, r *http.Request) {
	limit := 12
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 || parsed > 50 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 50")
			return
		}
		limit = parsed
	}

	query := r.URL.Query().Get("q")
	routes, err := a.store.SearchRoutes(r.Context(), query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search routes")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"query": query, "routes": routes})
}

func (a *App) handleRouteDetail(w http.ResponseWriter, r *http.Request) {
	routeID, err := strconv.ParseInt(r.PathValue("routeID"), 10, 64)
	if err != nil || routeID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid route id")
		return
	}

	route, err := a.store.GetRoute(r.Context(), routeID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load route")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"route": route})
}

func (a *App) handleRouteVisual(w http.ResponseWriter, r *http.Request) {
	routeID, err := strconv.ParseInt(r.PathValue("routeID"), 10, 64)
	if err != nil || routeID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid route id")
		return
	}

	route, err := a.store.GetRoute(r.Context(), routeID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load route")
		return
	}

	token, err := a.store.GetToken(r.Context(), route.AthleteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "missing oauth token")
		return
	}

	accessToken, err := a.ensureValidToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to refresh strava token")
		return
	}

	streams, err := a.strava.GetRouteStreams(r.Context(), accessToken, routeID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch route streams")
		return
	}

	writeJSON(w, http.StatusOK, routeVisualResponse{
		StreamPoints: buildVisualPoints(streams),
	})
}

func (a *App) handleSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	cleanPath := path.Clean(r.URL.Path)
	if cleanPath == "." || cleanPath == "/" {
		http.ServeFile(w, r, filepath.Join(a.staticDir, "index.html"))
		return
	}

	relative := strings.TrimPrefix(cleanPath, "/")
	candidate := filepath.Join(a.staticDir, relative)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		http.ServeFile(w, r, candidate)
		return
	}

	http.ServeFile(w, r, filepath.Join(a.staticDir, "index.html"))
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing session")
			return
		}

		athleteID, err := parseSessionCookie(a.cfg.SessionSecret, cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}

		next(w, r.WithContext(withAthleteID(r.Context(), athleteID)))
	}
}

func (a *App) ensureValidToken(ctx context.Context, token store.Token) (string, error) {
	if time.Until(token.ExpiresAt) > 90*time.Second {
		return token.AccessToken, nil
	}

	refreshed, err := a.strava.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return "", err
	}

	updated := store.Token{
		AthleteID:    token.AthleteID,
		AccessToken:  refreshed.AccessToken,
		RefreshToken: refreshed.RefreshToken,
		ExpiresAt:    time.Unix(refreshed.ExpiresAt, 0).UTC(),
		Scope:        token.Scope,
		TokenType:    refreshed.TokenType,
	}
	if persistErr := a.store.UpdateToken(ctx, updated); persistErr != nil {
		return "", persistErr
	}

	return updated.AccessToken, nil
}

func mapActivities(athleteID int64, items []strava.Activity) ([]store.Activity, error) {
	mapped := make([]store.Activity, 0, len(items))
	for _, item := range items {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}

		mapped = append(mapped, store.Activity{
			ID:                 item.ID,
			AthleteID:          athleteID,
			Name:               item.Name,
			Type:               item.Type,
			DistanceMeters:     item.Distance,
			MovingTimeSeconds:  item.MovingTime,
			ElapsedTimeSeconds: item.ElapsedTime,
			ElevationGain:      item.TotalElevationGain,
			AverageSpeed:       item.AverageSpeed,
			MaxSpeed:           item.MaxSpeed,
			StartDate:          item.StartDate,
			Timezone:           item.Timezone,
			MapPolyline:        item.Map.Polyline,
			SummaryPolyline:    item.Map.SummaryPolyline,
			Raw:                raw,
		})
	}
	return mapped, nil
}

func mapRoute(athleteID int64, route strava.Route) (store.Route, error) {
	raw, err := json.Marshal(route)
	if err != nil {
		return store.Route{}, err
	}

	return store.Route{
		ID:                  route.ID,
		AthleteID:           athleteID,
		Name:                route.Name,
		Description:         route.Description,
		DistanceMeters:      route.Distance,
		ElevationGain:       route.ElevationGain,
		EstimatedMovingTime: route.EstimatedMovingTime,
		MapPolyline:         route.Map.Polyline,
		SummaryPolyline:     route.Map.SummaryPolyline,
		OwnerFirstName:      route.Athlete.Firstname,
		OwnerLastName:       route.Athlete.Lastname,
		OwnerProfileMedium:  route.Athlete.ProfileMedium,
		OwnerProfile:        route.Athlete.Profile,
		Raw:                 raw,
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func bestPhotoURL(activity strava.DetailedActivity) string {
	if activity.Photos.Primary == nil {
		return ""
	}
	return activity.Photos.Primary.URLs.BestURL()
}

func thumbnailPhotoURL(activity strava.DetailedActivity) string {
	if activity.Photos.Primary == nil {
		return ""
	}
	return activity.Photos.Primary.URLs.ThumbnailURL()
}

func buildVisualPoints(streams strava.StreamSet) []visualPoint {
	if streams.Distance == nil || streams.Altitude == nil {
		return nil
	}

	count := len(streams.Distance.Data)
	if len(streams.Altitude.Data) < count {
		count = len(streams.Altitude.Data)
	}
	if count == 0 {
		return nil
	}

	hasLatLng := streams.LatLng != nil && len(streams.LatLng.Data) >= count
	points := make([]visualPoint, 0, count)
	for i := 0; i < count; i++ {
		point := visualPoint{
			DistanceMeters: streams.Distance.Data[i],
			AltitudeMeters: streams.Altitude.Data[i],
		}
		if hasLatLng && len(streams.LatLng.Data[i]) >= 2 {
			lat := streams.LatLng.Data[i][0]
			lng := streams.LatLng.Data[i][1]
			point.Lat = &lat
			point.Lng = &lng
		}
		points = append(points, point)
	}

	return points
}

func buildWeeklyTrendBuckets(activities []store.Activity, count int, now time.Time) []analyticsTrendBucketResponse {
	currentStart := startOfWeekUTC(now)
	buckets := make([]analyticsTrendBucketResponse, count)
	indexByKey := make(map[string]int, count)

	for index := 0; index < count; index++ {
		start := currentStart.AddDate(0, 0, -7*(count-1-index))
		end := start.AddDate(0, 0, 7)
		key := start.Format("2006-01-02")
		buckets[index] = analyticsTrendBucketResponse{
			Label: start.Format("Jan 02"),
			Start: start.Format(time.RFC3339),
			End:   end.Format(time.RFC3339),
		}
		indexByKey[key] = index
	}

	for _, activity := range activities {
		bucketStart := startOfWeekUTC(activity.StartDate.UTC())
		key := bucketStart.Format("2006-01-02")
		index, ok := indexByKey[key]
		if !ok {
			continue
		}

		buckets[index].ActivityCount++
		buckets[index].DistanceMeters += activity.DistanceMeters
		buckets[index].MovingTimeSeconds += activity.MovingTimeSeconds
		buckets[index].ElevationGain += activity.ElevationGain
	}

	finalizeTrendBuckets(buckets)
	return buckets
}

func buildMonthlyTrendBuckets(activities []store.Activity, count int, now time.Time) []analyticsTrendBucketResponse {
	currentStart := startOfMonthUTC(now)
	buckets := make([]analyticsTrendBucketResponse, count)
	indexByKey := make(map[string]int, count)

	for index := 0; index < count; index++ {
		start := currentStart.AddDate(0, -(count - 1 - index), 0)
		end := start.AddDate(0, 1, 0)
		key := start.Format("2006-01")
		buckets[index] = analyticsTrendBucketResponse{
			Label: start.Format("Jan"),
			Start: start.Format(time.RFC3339),
			End:   end.Format(time.RFC3339),
		}
		indexByKey[key] = index
	}

	for _, activity := range activities {
		bucketStart := startOfMonthUTC(activity.StartDate.UTC())
		key := bucketStart.Format("2006-01")
		index, ok := indexByKey[key]
		if !ok {
			continue
		}

		buckets[index].ActivityCount++
		buckets[index].DistanceMeters += activity.DistanceMeters
		buckets[index].MovingTimeSeconds += activity.MovingTimeSeconds
		buckets[index].ElevationGain += activity.ElevationGain
	}

	finalizeTrendBuckets(buckets)
	return buckets
}

func finalizeTrendBuckets(buckets []analyticsTrendBucketResponse) {
	for index := range buckets {
		if buckets[index].DistanceMeters <= 0 || buckets[index].MovingTimeSeconds <= 0 {
			continue
		}

		paceSecondsPerKm := float64(buckets[index].MovingTimeSeconds) / (buckets[index].DistanceMeters / 1000.0)
		buckets[index].AveragePaceSecondsPerKm = &paceSecondsPerKm
	}
}

func buildAnalyticsRecords(activities []store.Activity) analyticsRecordsResponse {
	var bestPace *float64
	records := analyticsRecordsResponse{}

	for _, activity := range activities {
		if activity.DistanceMeters > 0 && activity.MovingTimeSeconds > 0 {
			paceSecondsPerKm := float64(activity.MovingTimeSeconds) / (activity.DistanceMeters / 1000.0)
			if bestPace == nil || paceSecondsPerKm < *bestPace {
				pace := paceSecondsPerKm
				bestPace = &pace
			}
		}

		if activity.ElevationGain > records.MostElevationGain {
			records.MostElevationGain = activity.ElevationGain
		}
		if activity.MaxSpeed > records.HighestMaxSpeed {
			records.HighestMaxSpeed = activity.MaxSpeed
		}
		if activity.MovingTimeSeconds > records.LongestDuration {
			records.LongestDuration = activity.MovingTimeSeconds
		}
	}

	records.BestPaceSecondsPerKm = bestPace
	return records
}

func startOfWeekUTC(value time.Time) time.Time {
	current := value.UTC()
	year, month, day := current.Date()
	midnight := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	weekday := midnight.Weekday()
	delta := 0
	if weekday == time.Sunday {
		delta = -6
	} else {
		delta = -(int(weekday) - 1)
	}
	return midnight.AddDate(0, 0, delta)
}

func startOfMonthUTC(value time.Time) time.Time {
	current := value.UTC()
	year, month, _ := current.Date()
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}
