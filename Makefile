.PHONY: dev-backend dev-frontend build-frontend

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm install && npm run dev

build-frontend:
	cd frontend && npm install && npm run build
