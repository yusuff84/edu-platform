FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /api ./cmd/api

FROM alpine:3.20
RUN adduser -D -H appuser
USER appuser
COPY --from=build /api /api
COPY --from=build /app/internal/migrate/migrations /migrations
EXPOSE 8080
ENV PORT=8080
CMD ["/api"]
