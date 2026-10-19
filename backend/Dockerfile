FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /api ./cmd/api

FROM alpine:3.20
# uploads — каталог, который раздаётся маршрутом /uploads/* (T54: обложки
# курсов; storage.LocalStore по умолчанию пишет в ./uploads). Без этого каталога
# процесс appuser (uid 1000) не мог создать его в корне /, и загрузка обложек
# падала с 500 (найдено сквозным QA волны P7 на стенде).
RUN adduser -D -H appuser \
    && mkdir -p /uploads/covers \
    && chown -R appuser:appuser /uploads
USER appuser
COPY --from=build /api /api
COPY --from=build /app/internal/migrate/migrations /migrations
EXPOSE 8080
ENV PORT=8080
CMD ["/api"]
