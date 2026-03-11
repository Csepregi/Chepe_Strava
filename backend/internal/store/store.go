package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var ErrNotFound = errors.New("item not found")

type Store struct {
	client              *dynamodb.Client
	athletesTable       string
	activitiesTable     string
	activitiesByDateGSI string
	routesTable         string
}

type Athlete struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	FirstName     string `json:"firstname"`
	LastName      string `json:"lastname"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	ProfileMedium string `json:"profile_medium"`
	Profile       string `json:"profile"`
}

type Token struct {
	AthleteID    int64     `json:"athlete_id"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
	TokenType    string    `json:"token_type"`
}

type Activity struct {
	ID                 int64           `json:"id"`
	AthleteID          int64           `json:"athlete_id"`
	Name               string          `json:"name"`
	Type               string          `json:"type"`
	DistanceMeters     float64         `json:"distance_meters"`
	MovingTimeSeconds  int             `json:"moving_time_seconds"`
	ElapsedTimeSeconds int             `json:"elapsed_time_seconds"`
	ElevationGain      float64         `json:"total_elevation_gain"`
	AverageSpeed       float64         `json:"average_speed"`
	MaxSpeed           float64         `json:"max_speed"`
	StartDate          time.Time       `json:"start_date"`
	Timezone           string          `json:"timezone"`
	MapPolyline        string          `json:"map_polyline"`
	SummaryPolyline    string          `json:"summary_polyline"`
	Raw                json.RawMessage `json:"raw,omitempty"`
}

type Route struct {
	ID                  int64           `json:"id"`
	AthleteID           int64           `json:"athlete_id"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	DistanceMeters      float64         `json:"distance_meters"`
	ElevationGain       float64         `json:"elevation_gain"`
	EstimatedMovingTime int             `json:"estimated_moving_time"`
	MapPolyline         string          `json:"map_polyline"`
	SummaryPolyline     string          `json:"summary_polyline"`
	OwnerFirstName      string          `json:"owner_firstname"`
	OwnerLastName       string          `json:"owner_lastname"`
	OwnerProfileMedium  string          `json:"owner_profile_medium"`
	OwnerProfile        string          `json:"owner_profile"`
	Raw                 json.RawMessage `json:"raw,omitempty"`
}

type Summary struct {
	TotalActivities int     `json:"total_activities"`
	TotalDistanceKm float64 `json:"total_distance_km"`
	TotalMovingHrs  float64 `json:"total_moving_hours"`
	TotalElevation  float64 `json:"total_elevation_gain"`
}

type athleteItem struct {
	AthleteID          int64  `dynamodbav:"athlete_id"`
	Username           string `dynamodbav:"username,omitempty"`
	FirstName          string `dynamodbav:"firstname,omitempty"`
	LastName           string `dynamodbav:"lastname,omitempty"`
	City               string `dynamodbav:"city,omitempty"`
	State              string `dynamodbav:"state,omitempty"`
	Country            string `dynamodbav:"country,omitempty"`
	ProfileMedium      string `dynamodbav:"profile_medium,omitempty"`
	Profile            string `dynamodbav:"profile,omitempty"`
	AccessToken        string `dynamodbav:"access_token,omitempty"`
	RefreshToken       string `dynamodbav:"refresh_token,omitempty"`
	TokenExpiresAtUnix int64  `dynamodbav:"token_expires_at_unix"`
	TokenScope         string `dynamodbav:"token_scope,omitempty"`
	TokenType          string `dynamodbav:"token_type,omitempty"`
	UpdatedAtUnix      int64  `dynamodbav:"updated_at_unix"`
}

type activityItem struct {
	AthleteID          int64   `dynamodbav:"athlete_id"`
	ActivityID         int64   `dynamodbav:"activity_id"`
	StartEpoch         int64   `dynamodbav:"start_epoch"`
	StartDate          string  `dynamodbav:"start_date"`
	Name               string  `dynamodbav:"name,omitempty"`
	Type               string  `dynamodbav:"type,omitempty"`
	DistanceMeters     float64 `dynamodbav:"distance_meters"`
	MovingTimeSeconds  int     `dynamodbav:"moving_time_seconds"`
	ElapsedTimeSeconds int     `dynamodbav:"elapsed_time_seconds"`
	ElevationGain      float64 `dynamodbav:"total_elevation_gain"`
	AverageSpeed       float64 `dynamodbav:"average_speed"`
	MaxSpeed           float64 `dynamodbav:"max_speed"`
	Timezone           string  `dynamodbav:"timezone,omitempty"`
	MapPolyline        string  `dynamodbav:"map_polyline,omitempty"`
	SummaryPolyline    string  `dynamodbav:"summary_polyline,omitempty"`
	Raw                string  `dynamodbav:"raw,omitempty"`
}

type routeItem struct {
	RouteID             int64   `dynamodbav:"route_id"`
	AthleteID           int64   `dynamodbav:"athlete_id"`
	Name                string  `dynamodbav:"name,omitempty"`
	Description         string  `dynamodbav:"description,omitempty"`
	DistanceMeters      float64 `dynamodbav:"distance_meters"`
	ElevationGain       float64 `dynamodbav:"elevation_gain"`
	EstimatedMovingTime int     `dynamodbav:"estimated_moving_time"`
	MapPolyline         string  `dynamodbav:"map_polyline,omitempty"`
	SummaryPolyline     string  `dynamodbav:"summary_polyline,omitempty"`
	OwnerFirstName      string  `dynamodbav:"owner_firstname,omitempty"`
	OwnerLastName       string  `dynamodbav:"owner_lastname,omitempty"`
	OwnerProfileMedium  string  `dynamodbav:"owner_profile_medium,omitempty"`
	OwnerProfile        string  `dynamodbav:"owner_profile,omitempty"`
	UpdatedAtUnix       int64   `dynamodbav:"updated_at_unix"`
	Raw                 string  `dynamodbav:"raw,omitempty"`
}

func New(client *dynamodb.Client, athletesTable, activitiesTable, activitiesByDateGSI, routesTable string) *Store {
	return &Store{
		client:              client,
		athletesTable:       athletesTable,
		activitiesTable:     activitiesTable,
		activitiesByDateGSI: activitiesByDateGSI,
		routesTable:         routesTable,
	}
}

func (s *Store) UpsertAthleteAndToken(ctx context.Context, athlete Athlete, token Token) error {
	item := athleteItem{
		AthleteID:          athlete.ID,
		Username:           athlete.Username,
		FirstName:          athlete.FirstName,
		LastName:           athlete.LastName,
		City:               athlete.City,
		State:              athlete.State,
		Country:            athlete.Country,
		ProfileMedium:      athlete.ProfileMedium,
		Profile:            athlete.Profile,
		AccessToken:        token.AccessToken,
		RefreshToken:       token.RefreshToken,
		TokenExpiresAtUnix: token.ExpiresAt.Unix(),
		TokenScope:         token.Scope,
		TokenType:          token.TokenType,
		UpdatedAtUnix:      time.Now().Unix(),
	}

	attributeMap, err := attributevalue.MarshalMap(item)
	if err != nil {
		return fmt.Errorf("marshal athlete item: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.athletesTable),
		Item:      attributeMap,
	})
	if err != nil {
		return fmt.Errorf("put athlete item: %w", err)
	}

	return nil
}

func (s *Store) GetAthlete(ctx context.Context, athleteID int64) (Athlete, error) {
	item, err := s.getAthleteItem(ctx, athleteID)
	if err != nil {
		return Athlete{}, err
	}

	return Athlete{
		ID:            item.AthleteID,
		Username:      item.Username,
		FirstName:     item.FirstName,
		LastName:      item.LastName,
		City:          item.City,
		State:         item.State,
		Country:       item.Country,
		ProfileMedium: item.ProfileMedium,
		Profile:       item.Profile,
	}, nil
}

func (s *Store) GetToken(ctx context.Context, athleteID int64) (Token, error) {
	item, err := s.getAthleteItem(ctx, athleteID)
	if err != nil {
		return Token{}, err
	}

	if item.AccessToken == "" || item.RefreshToken == "" || item.TokenExpiresAtUnix == 0 {
		return Token{}, ErrNotFound
	}

	return Token{
		AthleteID:    item.AthleteID,
		AccessToken:  item.AccessToken,
		RefreshToken: item.RefreshToken,
		ExpiresAt:    time.Unix(item.TokenExpiresAtUnix, 0).UTC(),
		Scope:        item.TokenScope,
		TokenType:    item.TokenType,
	}, nil
}

func (s *Store) UpdateToken(ctx context.Context, token Token) error {
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.athletesTable),
		Key:       athleteKey(token.AthleteID),
		UpdateExpression: aws.String(
			"SET access_token = :access, refresh_token = :refresh, token_expires_at_unix = :expires, token_scope = :scope, token_type = :tokenType, updated_at_unix = :updated",
		),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":access":    &types.AttributeValueMemberS{Value: token.AccessToken},
			":refresh":   &types.AttributeValueMemberS{Value: token.RefreshToken},
			":expires":   &types.AttributeValueMemberN{Value: strconv.FormatInt(token.ExpiresAt.Unix(), 10)},
			":scope":     &types.AttributeValueMemberS{Value: token.Scope},
			":tokenType": &types.AttributeValueMemberS{Value: token.TokenType},
			":updated":   &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().Unix(), 10)},
		},
		ConditionExpression: aws.String("attribute_exists(athlete_id)"),
	})
	if err != nil {
		var conditionalErr *types.ConditionalCheckFailedException
		if errors.As(err, &conditionalErr) {
			return ErrNotFound
		}
		return fmt.Errorf("update token: %w", err)
	}

	return nil
}

func (s *Store) UpsertActivities(ctx context.Context, athleteID int64, activities []Activity) error {
	for _, activity := range activities {
		raw := ""
		if len(activity.Raw) > 0 {
			raw = string(activity.Raw)
		}

		item := activityItem{
			AthleteID:          athleteID,
			ActivityID:         activity.ID,
			StartEpoch:         activity.StartDate.Unix(),
			StartDate:          activity.StartDate.UTC().Format(time.RFC3339),
			Name:               activity.Name,
			Type:               activity.Type,
			DistanceMeters:     activity.DistanceMeters,
			MovingTimeSeconds:  activity.MovingTimeSeconds,
			ElapsedTimeSeconds: activity.ElapsedTimeSeconds,
			ElevationGain:      activity.ElevationGain,
			AverageSpeed:       activity.AverageSpeed,
			MaxSpeed:           activity.MaxSpeed,
			Timezone:           activity.Timezone,
			MapPolyline:        activity.MapPolyline,
			SummaryPolyline:    activity.SummaryPolyline,
			Raw:                raw,
		}

		attributeMap, err := attributevalue.MarshalMap(item)
		if err != nil {
			return fmt.Errorf("marshal activity %d: %w", activity.ID, err)
		}

		_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(s.activitiesTable),
			Item:      attributeMap,
		})
		if err != nil {
			return fmt.Errorf("put activity %d: %w", activity.ID, err)
		}
	}

	return nil
}

func (s *Store) ListActivities(ctx context.Context, athleteID int64, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 50
	}

	activities := make([]Activity, 0, limit)
	query := &dynamodb.QueryInput{
		TableName:              aws.String(s.activitiesTable),
		IndexName:              aws.String(s.activitiesByDateGSI),
		KeyConditionExpression: aws.String("athlete_id = :athlete"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":athlete": &types.AttributeValueMemberN{Value: strconv.FormatInt(athleteID, 10)},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(int32(limit)),
	}

	for {
		output, err := s.client.Query(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("query activities: %w", err)
		}

		var batch []activityItem
		if err := attributevalue.UnmarshalListOfMaps(output.Items, &batch); err != nil {
			return nil, fmt.Errorf("unmarshal activities: %w", err)
		}

		for _, item := range batch {
			activities = append(activities, convertActivity(item))
			if len(activities) >= limit {
				break
			}
		}

		if len(activities) >= limit || len(output.LastEvaluatedKey) == 0 {
			break
		}

		query.ExclusiveStartKey = output.LastEvaluatedKey
		query.Limit = aws.Int32(int32(limit - len(activities)))
	}

	sort.Slice(activities, func(i, j int) bool {
		return activities[i].StartDate.After(activities[j].StartDate)
	})

	if len(activities) > limit {
		activities = activities[:limit]
	}

	return activities, nil
}

func (s *Store) Summary(ctx context.Context, athleteID int64, days int) (Summary, error) {
	afterEpoch := time.Now().AddDate(0, 0, -days).Unix()
	query := &dynamodb.QueryInput{
		TableName:              aws.String(s.activitiesTable),
		IndexName:              aws.String(s.activitiesByDateGSI),
		KeyConditionExpression: aws.String("athlete_id = :athlete AND start_epoch >= :after"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":athlete": &types.AttributeValueMemberN{Value: strconv.FormatInt(athleteID, 10)},
			":after":   &types.AttributeValueMemberN{Value: strconv.FormatInt(afterEpoch, 10)},
		},
	}

	var summary Summary
	for {
		output, err := s.client.Query(ctx, query)
		if err != nil {
			return Summary{}, fmt.Errorf("query summary: %w", err)
		}

		var batch []activityItem
		if err := attributevalue.UnmarshalListOfMaps(output.Items, &batch); err != nil {
			return Summary{}, fmt.Errorf("unmarshal summary activities: %w", err)
		}

		for _, item := range batch {
			summary.TotalActivities++
			summary.TotalDistanceKm += item.DistanceMeters / 1000.0
			summary.TotalMovingHrs += float64(item.MovingTimeSeconds) / 3600.0
			summary.TotalElevation += item.ElevationGain
		}

		if len(output.LastEvaluatedKey) == 0 {
			break
		}
		query.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return summary, nil
}

func (s *Store) LatestActivity(ctx context.Context) (Activity, error) {
	athleteIDs, err := s.listAthleteIDs(ctx)
	if err != nil {
		return Activity{}, err
	}
	if len(athleteIDs) == 0 {
		return Activity{}, ErrNotFound
	}

	var latest Activity
	found := false

	for _, athleteID := range athleteIDs {
		activity, activityErr := s.latestActivityForAthlete(ctx, athleteID)
		if activityErr != nil {
			if errors.Is(activityErr, ErrNotFound) {
				continue
			}
			return Activity{}, activityErr
		}

		if !found || activity.StartDate.After(latest.StartDate) {
			latest = activity
			found = true
		}
	}

	if !found {
		return Activity{}, ErrNotFound
	}

	return latest, nil
}

func (s *Store) UpsertRoutes(ctx context.Context, routes []Route) error {
	for _, route := range routes {
		raw := ""
		if len(route.Raw) > 0 {
			raw = string(route.Raw)
		}

		item := routeItem{
			RouteID:             route.ID,
			AthleteID:           route.AthleteID,
			Name:                route.Name,
			Description:         route.Description,
			DistanceMeters:      route.DistanceMeters,
			ElevationGain:       route.ElevationGain,
			EstimatedMovingTime: route.EstimatedMovingTime,
			MapPolyline:         route.MapPolyline,
			SummaryPolyline:     route.SummaryPolyline,
			OwnerFirstName:      route.OwnerFirstName,
			OwnerLastName:       route.OwnerLastName,
			OwnerProfileMedium:  route.OwnerProfileMedium,
			OwnerProfile:        route.OwnerProfile,
			UpdatedAtUnix:       time.Now().Unix(),
			Raw:                 raw,
		}

		attributeMap, err := attributevalue.MarshalMap(item)
		if err != nil {
			return fmt.Errorf("marshal route %d: %w", route.ID, err)
		}

		_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(s.routesTable),
			Item:      attributeMap,
		})
		if err != nil {
			return fmt.Errorf("put route %d: %w", route.ID, err)
		}
	}

	return nil
}

func (s *Store) SearchRoutes(ctx context.Context, query string, limit int) ([]Route, error) {
	if limit <= 0 {
		limit = 12
	}

	normalizedQuery := normalizeSearch(query)
	input := &dynamodb.ScanInput{
		TableName: aws.String(s.routesTable),
	}

	routes := make([]Route, 0, limit)
	for {
		output, err := s.client.Scan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("scan routes: %w", err)
		}

		var batch []routeItem
		if err := attributevalue.UnmarshalListOfMaps(output.Items, &batch); err != nil {
			return nil, fmt.Errorf("unmarshal routes: %w", err)
		}

		for _, item := range batch {
			if normalizedQuery != "" && !strings.Contains(routeSearchBlob(item), normalizedQuery) {
				continue
			}
			routes = append(routes, convertRoute(item))
		}

		if len(output.LastEvaluatedKey) == 0 {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	sort.Slice(routes, func(i, j int) bool {
		left := routeSearchRank(routes[i], normalizedQuery)
		right := routeSearchRank(routes[j], normalizedQuery)
		if left == right {
			return strings.ToLower(routes[i].Name) < strings.ToLower(routes[j].Name)
		}
		return left < right
	})

	if len(routes) > limit {
		routes = routes[:limit]
	}

	return routes, nil
}

func (s *Store) SearchActivities(ctx context.Context, query string, limit int) ([]Activity, error) {
	if limit <= 0 {
		limit = 12
	}

	normalizedQuery := normalizeSearch(query)
	input := &dynamodb.ScanInput{
		TableName: aws.String(s.activitiesTable),
	}

	activities := make([]Activity, 0, limit)
	for {
		output, err := s.client.Scan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("scan activities for search: %w", err)
		}

		var batch []activityItem
		if err := attributevalue.UnmarshalListOfMaps(output.Items, &batch); err != nil {
			return nil, fmt.Errorf("unmarshal activities for search: %w", err)
		}

		for _, item := range batch {
			if normalizedQuery != "" && !strings.Contains(activitySearchBlob(item), normalizedQuery) {
				continue
			}
			activities = append(activities, convertActivity(item))
		}

		if len(output.LastEvaluatedKey) == 0 {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	sort.Slice(activities, func(i, j int) bool {
		left := activitySearchRank(activities[i], normalizedQuery)
		right := activitySearchRank(activities[j], normalizedQuery)
		if left == right {
			return activities[i].StartDate.After(activities[j].StartDate)
		}
		return left < right
	})

	if len(activities) > limit {
		activities = activities[:limit]
	}

	return activities, nil
}

func (s *Store) GetRoute(ctx context.Context, routeID int64) (Route, error) {
	output, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.routesTable),
		Key:            routeKey(routeID),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return Route{}, fmt.Errorf("get route %d: %w", routeID, err)
	}
	if len(output.Item) == 0 {
		return Route{}, ErrNotFound
	}

	var item routeItem
	if err := attributevalue.UnmarshalMap(output.Item, &item); err != nil {
		return Route{}, fmt.Errorf("unmarshal route %d: %w", routeID, err)
	}

	return convertRoute(item), nil
}

func (s *Store) getAthleteItem(ctx context.Context, athleteID int64) (athleteItem, error) {
	output, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.athletesTable),
		Key:            athleteKey(athleteID),
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return athleteItem{}, fmt.Errorf("get athlete %d: %w", athleteID, err)
	}
	if len(output.Item) == 0 {
		return athleteItem{}, ErrNotFound
	}

	var item athleteItem
	if err := attributevalue.UnmarshalMap(output.Item, &item); err != nil {
		return athleteItem{}, fmt.Errorf("unmarshal athlete %d: %w", athleteID, err)
	}

	return item, nil
}

func athleteKey(athleteID int64) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"athlete_id": &types.AttributeValueMemberN{Value: strconv.FormatInt(athleteID, 10)},
	}
}

func routeKey(routeID int64) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"route_id": &types.AttributeValueMemberN{Value: strconv.FormatInt(routeID, 10)},
	}
}

func convertActivity(item activityItem) Activity {
	startDate := time.Unix(item.StartEpoch, 0).UTC()
	if parsed, err := time.Parse(time.RFC3339, item.StartDate); err == nil {
		startDate = parsed
	}

	var raw json.RawMessage
	if item.Raw != "" {
		raw = json.RawMessage(item.Raw)
	}

	return Activity{
		ID:                 item.ActivityID,
		AthleteID:          item.AthleteID,
		Name:               item.Name,
		Type:               item.Type,
		DistanceMeters:     item.DistanceMeters,
		MovingTimeSeconds:  item.MovingTimeSeconds,
		ElapsedTimeSeconds: item.ElapsedTimeSeconds,
		ElevationGain:      item.ElevationGain,
		AverageSpeed:       item.AverageSpeed,
		MaxSpeed:           item.MaxSpeed,
		StartDate:          startDate,
		Timezone:           item.Timezone,
		MapPolyline:        item.MapPolyline,
		SummaryPolyline:    item.SummaryPolyline,
		Raw:                raw,
	}
}

func convertRoute(item routeItem) Route {
	var raw json.RawMessage
	if item.Raw != "" {
		raw = json.RawMessage(item.Raw)
	}

	return Route{
		ID:                  item.RouteID,
		AthleteID:           item.AthleteID,
		Name:                item.Name,
		Description:         item.Description,
		DistanceMeters:      item.DistanceMeters,
		ElevationGain:       item.ElevationGain,
		EstimatedMovingTime: item.EstimatedMovingTime,
		MapPolyline:         item.MapPolyline,
		SummaryPolyline:     item.SummaryPolyline,
		OwnerFirstName:      item.OwnerFirstName,
		OwnerLastName:       item.OwnerLastName,
		OwnerProfileMedium:  item.OwnerProfileMedium,
		OwnerProfile:        item.OwnerProfile,
		Raw:                 raw,
	}
}

func normalizeSearch(input string) string {
	return strings.ToLower(strings.TrimSpace(input))
}

func routeSearchBlob(item routeItem) string {
	parts := []string{
		item.Name,
		item.Description,
		item.OwnerFirstName,
		item.OwnerLastName,
	}
	return normalizeSearch(strings.Join(parts, " "))
}

func routeSearchRank(route Route, normalizedQuery string) int {
	if normalizedQuery == "" {
		return 0
	}

	name := normalizeSearch(route.Name)
	description := normalizeSearch(route.Description)
	if strings.HasPrefix(name, normalizedQuery) {
		return 0
	}
	if strings.Contains(name, normalizedQuery) {
		return 1
	}
	if strings.Contains(description, normalizedQuery) {
		return 2
	}
	return 3
}

func activitySearchBlob(item activityItem) string {
	parts := []string{
		item.Name,
		item.Type,
		item.Timezone,
	}
	return normalizeSearch(strings.Join(parts, " "))
}

func activitySearchRank(activity Activity, normalizedQuery string) int {
	if normalizedQuery == "" {
		return 0
	}

	name := normalizeSearch(activity.Name)
	activityType := normalizeSearch(activity.Type)
	timezone := normalizeSearch(activity.Timezone)
	if strings.HasPrefix(name, normalizedQuery) {
		return 0
	}
	if strings.Contains(name, normalizedQuery) {
		return 1
	}
	if strings.Contains(activityType, normalizedQuery) {
		return 2
	}
	if strings.Contains(timezone, normalizedQuery) {
		return 3
	}
	return 4
}

func (s *Store) listAthleteIDs(ctx context.Context) ([]int64, error) {
	input := &dynamodb.ScanInput{
		TableName:            aws.String(s.athletesTable),
		ProjectionExpression: aws.String("athlete_id"),
	}

	ids := make([]int64, 0, 4)
	seen := make(map[int64]struct{})

	for {
		output, err := s.client.Scan(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("scan athletes: %w", err)
		}

		for _, item := range output.Items {
			var athlete struct {
				AthleteID int64 `dynamodbav:"athlete_id"`
			}
			if err := attributevalue.UnmarshalMap(item, &athlete); err != nil {
				return nil, fmt.Errorf("unmarshal athlete id: %w", err)
			}
			if _, ok := seen[athlete.AthleteID]; ok {
				continue
			}
			seen[athlete.AthleteID] = struct{}{}
			ids = append(ids, athlete.AthleteID)
		}

		if len(output.LastEvaluatedKey) == 0 {
			break
		}
		input.ExclusiveStartKey = output.LastEvaluatedKey
	}

	return ids, nil
}

func (s *Store) latestActivityForAthlete(ctx context.Context, athleteID int64) (Activity, error) {
	output, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.activitiesTable),
		IndexName:              aws.String(s.activitiesByDateGSI),
		KeyConditionExpression: aws.String("athlete_id = :athlete"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":athlete": &types.AttributeValueMemberN{Value: strconv.FormatInt(athleteID, 10)},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(1),
	})
	if err != nil {
		return Activity{}, fmt.Errorf("query latest activity for athlete %d: %w", athleteID, err)
	}
	if len(output.Items) == 0 {
		return Activity{}, ErrNotFound
	}

	var items []activityItem
	if err := attributevalue.UnmarshalListOfMaps(output.Items, &items); err != nil {
		return Activity{}, fmt.Errorf("unmarshal latest activity for athlete %d: %w", athleteID, err)
	}
	if len(items) == 0 {
		return Activity{}, ErrNotFound
	}

	return convertActivity(items[0]), nil
}
