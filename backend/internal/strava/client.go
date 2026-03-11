package strava

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	stravaOAuthAuthorizeURL = "https://www.strava.com/oauth/authorize"
	stravaOAuthTokenURL     = "https://www.strava.com/oauth/token"
	stravaActivitiesURL     = "https://www.strava.com/api/v3/athlete/activities"
	stravaAPIRootURL        = "https://www.strava.com/api/v3"
)

type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	redirectURI  string
	scopes       string
}

type Athlete struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	Firstname     string `json:"firstname"`
	Lastname      string `json:"lastname"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	ProfileMedium string `json:"profile_medium"`
	Profile       string `json:"profile"`
}

type TokenExchangeResponse struct {
	TokenType    string  `json:"token_type"`
	ExpiresAt    int64   `json:"expires_at"`
	ExpiresIn    int64   `json:"expires_in"`
	RefreshToken string  `json:"refresh_token"`
	AccessToken  string  `json:"access_token"`
	Athlete      Athlete `json:"athlete"`
}

type Activity struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	Type               string    `json:"type"`
	Distance           float64   `json:"distance"`
	MovingTime         int       `json:"moving_time"`
	ElapsedTime        int       `json:"elapsed_time"`
	TotalElevationGain float64   `json:"total_elevation_gain"`
	AverageSpeed       float64   `json:"average_speed"`
	MaxSpeed           float64   `json:"max_speed"`
	StartDate          time.Time `json:"start_date"`
	Timezone           string    `json:"timezone"`
	Map                Map       `json:"map"`
}

type DetailedActivity struct {
	Activity
	Description string        `json:"description"`
	Photos      PhotosSummary `json:"photos"`
}

type SummaryAthlete struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	Firstname     string `json:"firstname"`
	Lastname      string `json:"lastname"`
	ProfileMedium string `json:"profile_medium"`
	Profile       string `json:"profile"`
}

type Map struct {
	ID              string `json:"id"`
	Polyline        string `json:"polyline"`
	SummaryPolyline string `json:"summary_polyline"`
}

type Route struct {
	ID                  int64          `json:"id"`
	Athlete             SummaryAthlete `json:"athlete"`
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	Distance            float64        `json:"distance"`
	ElevationGain       float64        `json:"elevation_gain"`
	EstimatedMovingTime int            `json:"estimated_moving_time"`
	Map                 Map            `json:"map"`
}

type PhotosSummary struct {
	Count   int           `json:"count"`
	Primary *PhotoPrimary `json:"primary"`
}

type PhotoPrimary struct {
	UniqueID string    `json:"unique_id"`
	URLs     PhotoURLs `json:"urls"`
}

type PhotoURLs map[string]string

type FloatStream struct {
	OriginalSize int       `json:"original_size"`
	Resolution   string    `json:"resolution"`
	SeriesType   string    `json:"series_type"`
	Data         []float64 `json:"data"`
}

type LatLngStream struct {
	OriginalSize int         `json:"original_size"`
	Resolution   string      `json:"resolution"`
	SeriesType   string      `json:"series_type"`
	Data         [][]float64 `json:"data"`
}

type StreamSet struct {
	Distance *FloatStream  `json:"distance,omitempty"`
	Altitude *FloatStream  `json:"altitude,omitempty"`
	LatLng   *LatLngStream `json:"latlng,omitempty"`
}

type streamEnvelope struct {
	Type         string          `json:"type"`
	OriginalSize int             `json:"original_size"`
	Resolution   string          `json:"resolution"`
	SeriesType   string          `json:"series_type"`
	Data         json.RawMessage `json:"data"`
}

type tokenRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Code         string `json:"code,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	GrantType    string `json:"grant_type"`
}

func NewClient(clientID, clientSecret, redirectURI, scopes string) *Client {
	return &Client{
		httpClient:   &http.Client{Timeout: 20 * time.Second},
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		scopes:       scopes,
	}
}

func (c *Client) AuthURL(state string) string {
	params := url.Values{}
	params.Set("client_id", c.clientID)
	params.Set("redirect_uri", c.redirectURI)
	params.Set("response_type", "code")
	params.Set("approval_prompt", "auto")
	params.Set("scope", c.scopes)
	params.Set("state", state)

	return stravaOAuthAuthorizeURL + "?" + params.Encode()
}

func (c *Client) ExchangeCode(ctx context.Context, code string) (TokenExchangeResponse, error) {
	requestBody := tokenRequest{
		ClientID:     c.clientID,
		ClientSecret: c.clientSecret,
		Code:         code,
		GrantType:    "authorization_code",
	}

	return c.exchangeToken(ctx, requestBody)
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (TokenExchangeResponse, error) {
	requestBody := tokenRequest{
		ClientID:     c.clientID,
		ClientSecret: c.clientSecret,
		RefreshToken: refreshToken,
		GrantType:    "refresh_token",
	}

	return c.exchangeToken(ctx, requestBody)
}

func (c *Client) exchangeToken(ctx context.Context, payload tokenRequest) (TokenExchangeResponse, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return TokenExchangeResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stravaOAuthTokenURL, bytes.NewReader(encoded))
	if err != nil {
		return TokenExchangeResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TokenExchangeResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return TokenExchangeResponse{}, fmt.Errorf("strava token exchange failed: %d %s", resp.StatusCode, string(body))
	}

	var parsed TokenExchangeResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&parsed); decodeErr != nil {
		return TokenExchangeResponse{}, decodeErr
	}

	return parsed, nil
}

func (c *Client) FetchActivities(ctx context.Context, accessToken string, perPage, page int, after time.Time) ([]Activity, error) {
	requestURL, err := url.Parse(stravaActivitiesURL)
	if err != nil {
		return nil, err
	}

	query := requestURL.Query()
	query.Set("per_page", strconv.Itoa(perPage))
	query.Set("page", strconv.Itoa(page))
	query.Set("after", strconv.FormatInt(after.Unix(), 10))
	requestURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("strava activity fetch failed: %d %s", resp.StatusCode, string(body))
	}

	var activities []Activity
	if decodeErr := json.NewDecoder(resp.Body).Decode(&activities); decodeErr != nil {
		return nil, decodeErr
	}

	return activities, nil
}

func (c *Client) GetActivityByID(ctx context.Context, accessToken string, activityID int64) (DetailedActivity, error) {
	requestURL := fmt.Sprintf("%s/activities/%d", stravaAPIRootURL, activityID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return DetailedActivity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DetailedActivity{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return DetailedActivity{}, fmt.Errorf("strava activity detail fetch failed: %d %s", resp.StatusCode, string(body))
	}

	var activity DetailedActivity
	if decodeErr := json.NewDecoder(resp.Body).Decode(&activity); decodeErr != nil {
		return DetailedActivity{}, decodeErr
	}

	return activity, nil
}

func (c *Client) GetActivityStreams(ctx context.Context, accessToken string, activityID int64) (StreamSet, error) {
	requestURL, err := url.Parse(fmt.Sprintf("%s/activities/%d/streams", stravaAPIRootURL, activityID))
	if err != nil {
		return StreamSet{}, err
	}

	query := requestURL.Query()
	query.Set("keys", "distance,altitude,latlng")
	query.Set("key_by_type", "true")
	requestURL.RawQuery = query.Encode()

	return c.fetchStreamSet(ctx, accessToken, requestURL.String(), "strava activity streams fetch failed")
}

func (c *Client) ListAthleteRoutes(ctx context.Context, accessToken string, athleteID int64) ([]Route, error) {
	requestURL := fmt.Sprintf("%s/athletes/%d/routes", stravaAPIRootURL, athleteID)
	return c.getRoutes(ctx, accessToken, requestURL)
}

func (c *Client) GetRouteByID(ctx context.Context, accessToken string, routeID int64) (Route, error) {
	requestURL := fmt.Sprintf("%s/routes/%d", stravaAPIRootURL, routeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return Route{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Route{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return Route{}, fmt.Errorf("strava route fetch failed: %d %s", resp.StatusCode, string(body))
	}

	var route Route
	if decodeErr := json.NewDecoder(resp.Body).Decode(&route); decodeErr != nil {
		return Route{}, decodeErr
	}

	return route, nil
}

func (c *Client) GetRouteStreams(ctx context.Context, accessToken string, routeID int64) (StreamSet, error) {
	requestURL := fmt.Sprintf("%s/routes/%d/streams", stravaAPIRootURL, routeID)
	return c.fetchStreamSet(ctx, accessToken, requestURL, "strava route streams fetch failed")
}

func (c *Client) getRoutes(ctx context.Context, accessToken string, requestURL string) ([]Route, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("strava route list failed: %d %s", resp.StatusCode, string(body))
	}

	var routes []Route
	if decodeErr := json.NewDecoder(resp.Body).Decode(&routes); decodeErr != nil {
		return nil, decodeErr
	}

	return routes, nil
}

func (c *Client) fetchStreamSet(ctx context.Context, accessToken string, requestURL, errorPrefix string) (StreamSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return StreamSet{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return StreamSet{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return StreamSet{}, fmt.Errorf("%s: %d %s", errorPrefix, resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return StreamSet{}, err
	}

	return decodeStreamSet(body)
}

func decodeStreamSet(body []byte) (StreamSet, error) {
	var direct StreamSet
	if err := json.Unmarshal(body, &direct); err == nil {
		if direct.Distance != nil || direct.Altitude != nil || direct.LatLng != nil {
			return direct, nil
		}
	}

	var envelopes []streamEnvelope
	if err := json.Unmarshal(body, &envelopes); err != nil {
		return StreamSet{}, err
	}

	var set StreamSet
	for _, envelope := range envelopes {
		switch envelope.Type {
		case "distance":
			stream := FloatStream{
				OriginalSize: envelope.OriginalSize,
				Resolution:   envelope.Resolution,
				SeriesType:   envelope.SeriesType,
			}
			if err := json.Unmarshal(envelope.Data, &stream.Data); err != nil {
				return StreamSet{}, err
			}
			set.Distance = &stream
		case "altitude":
			stream := FloatStream{
				OriginalSize: envelope.OriginalSize,
				Resolution:   envelope.Resolution,
				SeriesType:   envelope.SeriesType,
			}
			if err := json.Unmarshal(envelope.Data, &stream.Data); err != nil {
				return StreamSet{}, err
			}
			set.Altitude = &stream
		case "latlng":
			stream := LatLngStream{
				OriginalSize: envelope.OriginalSize,
				Resolution:   envelope.Resolution,
				SeriesType:   envelope.SeriesType,
			}
			if err := json.Unmarshal(envelope.Data, &stream.Data); err != nil {
				return StreamSet{}, err
			}
			set.LatLng = &stream
		}
	}

	return set, nil
}

func (u *PhotoURLs) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*u = nil
		return nil
	}

	var mapping map[string]string
	if err := json.Unmarshal(data, &mapping); err == nil {
		*u = mapping
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*u = PhotoURLs{"default": single}
		return nil
	}

	return fmt.Errorf("unsupported photo urls payload")
}

func (u PhotoURLs) BestURL() string {
	for _, key := range []string{"600", "default", "300", "100"} {
		if url := u[key]; url != "" {
			return url
		}
	}

	for _, url := range u {
		if url != "" {
			return url
		}
	}

	return ""
}

func (u PhotoURLs) ThumbnailURL() string {
	for _, key := range []string{"100", "300", "default", "600"} {
		if url := u[key]; url != "" {
			return url
		}
	}

	for _, url := range u {
		if url != "" {
			return url
		}
	}

	return ""
}
