FROM golang:1.24-bookworm AS build
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN go build -o /runner ./...

FROM debian:bookworm-slim
# Сеть для контейнера отключается на уровне compose-профиля/файрвола (no-net).
RUN apt-get update && apt-get install -y --no-install-recommends python3 nodejs ca-certificates \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /runner /usr/local/bin/runner
EXPOSE 8081
CMD ["runner"]
