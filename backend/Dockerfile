FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/hirepilot ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/hirepilot /usr/local/bin/hirepilot
COPY migrations ./migrations
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/hirepilot"]
