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
