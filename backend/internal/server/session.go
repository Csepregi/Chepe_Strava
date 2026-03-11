package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName = "strava_session"
	stateCookieName   = "strava_oauth_state"
)

type contextKey string

const athleteIDContextKey contextKey = "athlete_id"

func makeSessionCookie(secret string, athleteID int64, ttl time.Duration, secure bool) (*http.Cookie, error) {
	expires := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%d:%d", athleteID, expires)
	signature := sign(payload, secret)
	value := payload + ":" + hex.EncodeToString(signature)

	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	}, nil
}

func clearSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

func parseSessionCookie(secret string, cookieValue string) (int64, error) {
	parts := strings.Split(cookieValue, ":")
	if len(parts) != 3 {
		return 0, errors.New("invalid session format")
	}

	athleteID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, errors.New("invalid athlete id")
	}

	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, errors.New("invalid expiry")
	}

	receivedSig, err := hex.DecodeString(parts[2])
	if err != nil {
		return 0, errors.New("invalid signature")
	}

	payload := parts[0] + ":" + parts[1]
	expectedSig := sign(payload, secret)

	if subtle.ConstantTimeCompare(receivedSig, expectedSig) != 1 {
		return 0, errors.New("signature mismatch")
	}

	if time.Now().Unix() > expires {
		return 0, errors.New("session expired")
	}

	return athleteID, nil
}

func createStateCookie(secure bool) (*http.Cookie, string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return nil, "", err
	}

	state := base64.RawURLEncoding.EncodeToString(bytes)
	cookie := &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	}

	return cookie, state, nil
}

func clearStateCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

func sign(payload string, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func withAthleteID(ctx context.Context, athleteID int64) context.Context {
	return context.WithValue(ctx, athleteIDContextKey, athleteID)
}

func athleteIDFromContext(ctx context.Context) (int64, bool) {
	athleteID, ok := ctx.Value(athleteIDContextKey).(int64)
	return athleteID, ok
}
