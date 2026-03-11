# syntax=docker/dockerfile:1

FROM node:22-alpine AS web-build
WORKDIR /web
ARG VITE_MAPBOX_ACCESS_TOKEN=""
ENV VITE_MAPBOX_ACCESS_TOKEN=$VITE_MAPBOX_ACCESS_TOKEN
COPY frontend/package.json frontend/package.json
COPY frontend/tsconfig.json frontend/tsconfig.json
COPY frontend/tsconfig.app.json frontend/tsconfig.app.json
COPY frontend/tsconfig.node.json frontend/tsconfig.node.json
COPY frontend/vite.config.ts frontend/vite.config.ts
COPY frontend/index.html frontend/index.html
COPY frontend/src frontend/src
RUN cd frontend && npm install && npm run build

FROM golang:1.24-alpine AS api-build
WORKDIR /src
COPY backend/go.mod backend/go.mod
RUN cd backend && go mod download
COPY backend backend
RUN cd backend && go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=api-build /out/server /app/server
COPY --from=web-build /web/frontend/dist /app/frontend/dist
ENV APP_ENV=production
ENV PORT=8080
ENV STATIC_DIR=/app/frontend/dist
EXPOSE 8080
CMD ["/app/server"]
