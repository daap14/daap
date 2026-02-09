# ============================================
# Stage: dev — hot-reload development image
# ============================================
FROM golang:1.25-alpine AS dev

RUN apk add --no-cache git curl

RUN go install github.com/air-verse/air@latest

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

EXPOSE 8080

CMD ["air"]

# ============================================
# Stage: build — compile production binary
# ============================================
FROM golang:1.25-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /daap ./cmd/server

# ============================================
# Stage: prod — minimal production image
# ============================================
FROM alpine:3.20 AS prod

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S daap && adduser -S daap -G daap

WORKDIR /app

COPY --from=build /daap /app/daap

USER daap

EXPOSE 8080

ENTRYPOINT ["/app/daap"]
