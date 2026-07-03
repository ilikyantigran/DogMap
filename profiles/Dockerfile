# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /bin/app ./cmd/profiles

# --- run stage ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /bin/app /app/app
COPY --from=build /src/configs /app/configs
ENV CONFIG_PATH=/app/configs/values_docker.yaml
EXPOSE 8082 8083
ENTRYPOINT ["/app/app"]
