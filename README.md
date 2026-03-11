# Strava Analytics App (React + Go + DynamoDB)

Strava analytics app with:

- React frontend dashboard
- Go backend API
- Strava OAuth login
- AWS DynamoDB storage for athletes, activities, and cached routes
- Fly.io deployment
- Public route search backed by synced Strava routes

## Project Layout

- `frontend/`: React + Vite app
- `backend/`: Go API (OAuth, sessions, activity sync, route sync, public route search)
- `fly.toml`: Fly deploy config
- `Dockerfile`: multi-stage build

## API Endpoints

- `GET /health`
- `GET /api/auth/strava/login`
- `GET /api/auth/strava/callback`
- `POST /api/auth/logout`
- `GET /api/me`
- `POST /api/activities/sync`
- `GET /api/activities?limit=80`
- `GET /api/analytics/summary?days=90`
- `POST /api/routes/sync`
- `GET /api/routes/search?q=alp&limit=12`
- `GET /api/routes/{routeID}`

## 1) Create Strava App

1. Open https://www.strava.com/settings/api
2. Create the app.
3. Set **Authorization Callback Domain**:
   - Local: `localhost`
   - Production: `<your-fly-app>.fly.dev`
4. Keep `Client ID` and `Client Secret`.

## 2) Create DynamoDB Tables

The examples below use `eu-central-1`. Keep your app config on the same region.

Create athletes table:

```bash
aws dynamodb create-table \
  --table-name strava-athletes \
  --attribute-definitions AttributeName=athlete_id,AttributeType=N \
  --key-schema AttributeName=athlete_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --region eu-central-1
```

Create activities table:

```bash
aws dynamodb create-table \
  --table-name strava-activities \
  --attribute-definitions \
    AttributeName=athlete_id,AttributeType=N \
    AttributeName=activity_id,AttributeType=N \
    AttributeName=start_epoch,AttributeType=N \
  --key-schema \
    AttributeName=athlete_id,KeyType=HASH \
    AttributeName=activity_id,KeyType=RANGE \
  --global-secondary-indexes '[
    {
      "IndexName": "athlete_start_idx",
      "KeySchema": [
        {"AttributeName":"athlete_id","KeyType":"HASH"},
        {"AttributeName":"start_epoch","KeyType":"RANGE"}
      ],
      "Projection": {"ProjectionType":"ALL"}
    }
  ]' \
  --billing-mode PAY_PER_REQUEST \
  --region eu-central-1
```

Create routes table:

```bash
aws dynamodb create-table \
  --table-name strava-routes \
  --attribute-definitions AttributeName=route_id,AttributeType=N \
  --key-schema AttributeName=route_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --region eu-central-1
```

## 3) IAM Access For The App

The Fly app needs these DynamoDB actions:

- `dynamodb:GetItem`
- `dynamodb:PutItem`
- `dynamodb:UpdateItem`
- `dynamodb:Query`
- `dynamodb:Scan`

Example inline policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "dynamodb:GetItem",
        "dynamodb:PutItem",
        "dynamodb:UpdateItem",
        "dynamodb:Query",
        "dynamodb:Scan"
      ],
      "Resource": [
        "arn:aws:dynamodb:eu-central-1:*:table/strava-athletes",
        "arn:aws:dynamodb:eu-central-1:*:table/strava-activities",
        "arn:aws:dynamodb:eu-central-1:*:table/strava-activities/index/athlete_start_idx",
        "arn:aws:dynamodb:eu-central-1:*:table/strava-routes"
      ]
    }
  ]
}
```

If your org uses explicit deny or MFA-enforced policies, an admin may need to create the service user or access key for you.

## 4) Local Environment

```bash
cp backend/.env.example backend/.env
```

Set these values in `backend/.env`:

- `SESSION_SECRET`
- `STRAVA_CLIENT_ID`
- `STRAVA_CLIENT_SECRET`
- `STRAVA_REDIRECT_URL=http://localhost:8080/api/auth/strava/callback`
- `AWS_REGION=eu-central-1`
- `DDB_ATHLETES_TABLE=strava-athletes`
- `DDB_ACTIVITIES_TABLE=strava-activities`
- `DDB_ACTIVITIES_BY_DATE_INDEX=athlete_start_idx`
- `DDB_ROUTES_TABLE=strava-routes`
- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`

Optional frontend env:

```bash
cp frontend/.env.example frontend/.env
```

Add your Mapbox public token to `frontend/.env`:

```bash
VITE_MAPBOX_ACCESS_TOKEN=pk.your_mapbox_token
```

## 5) Run Locally

Backend:

```bash
make dev-backend
```

Frontend:

```bash
make dev-frontend
```

Open: http://localhost:5173

After logging in:

1. Sync activities if you want analytics populated.
2. Sync routes if you want the public route search to show results.

## 6) Deploy To Fly.io

Create the app:

```bash
fly auth login
fly launch --no-deploy
```

Set secrets:

```bash
fly secrets set \
  SESSION_SECRET='your-random-secret' \
  STRAVA_CLIENT_ID='...' \
  STRAVA_CLIENT_SECRET='...' \
  STRAVA_REDIRECT_URL='https://<your-app>.fly.dev/api/auth/strava/callback' \
  FRONTEND_URL='https://<your-app>.fly.dev' \
  AWS_ACCESS_KEY_ID='...' \
  AWS_SECRET_ACCESS_KEY='...'
```

Deploy:

```bash
fly deploy --build-arg VITE_MAPBOX_ACCESS_TOKEN=pk.your_mapbox_token
```

## Notes

- Strava route search in this app is based on cached routes you sync into DynamoDB.
- The Strava route endpoints expose route geometry and athlete profile images, but not a per-route photo gallery.
- The Mapbox token is used only in the frontend build and is safe to treat as a public token.
- Strava access and refresh tokens are stored after OAuth login. They do not belong in `.env`.
- Rotating `SESSION_SECRET` logs all users out.
