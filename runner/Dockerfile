# =============================================================================
# Runner — изолированное выполнение учебного кода.
#
# Многоэтапная сборка:
#   1) build — только компилятор Go; здесь же гоняются go vet и тесты.
#      python3/nodejs ставятся в этот этап, чтобы тесты на реальный запуск
#      интерпретаторов выполнялись при каждой сборке (в чистом golang-образе
#      их нет, и тесты бы просто пропускались).
#   2) финальный образ — только рантайм (python3, nodejs) и бинарник;
#      работает от непривилегированного пользователя runner (uid 10001).
#
# Сеть, память, CPU и права контейнера НЕ задаются здесь — их задаёт
# docker-compose.yml (см. рекомендованный блок в отчёте T11):
# internal-сеть без выхода в интернет, read_only, cap_drop: [ALL],
# tmpfs на /tmp, лимиты cpu/memory, pids_limit.
# =============================================================================

# ---- Этап 1: сборка и тесты -------------------------------------------------
FROM golang:1.24-bookworm AS build
WORKDIR /src

# Интерпретаторы — только для тестов на этом этапе.
RUN apt-get update && apt-get install -y --no-install-recommends python3 nodejs \
  && rm -rf /var/lib/apt/lists/*

# go.mod без зависимостей: копируем раньше исходников — слой кэшируется.
COPY go.mod ./
COPY main.go main_test.go ./

RUN go vet ./... && go test ./... -count=1

# Статическая сборка: бинарник работает на чистом debian без libc-сюрпризов.
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/runner .

# ---- Этап 2: рантайм --------------------------------------------------------
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends python3 nodejs \
  && rm -rf /var/lib/apt/lists/* \
  && groupadd --gid 10001 runner \
  && useradd --uid 10001 --gid runner --home-dir /tmp --shell /usr/sbin/nologin runner \
  && chmod 1777 /tmp

COPY --from=build /out/runner /usr/local/bin/runner

ENV HOME=/tmp \
    LANG=C.UTF-8

# От root: все дочерние процессы (включая код ученика) идут от runner (10001).
USER runner:runner
# /tmp — рабочий каталог: сюда пишется код ученика (в compose монтируется tmpfs).
WORKDIR /tmp

EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/runner"]
