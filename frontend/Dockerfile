# Фронтенд-сборка нарочно без директивы `# syntax=docker/dockerfile:1`:
# вся здесь используемая грамматика (AS / COPY --from / EXPOSE / CMD) есть у
# встроенного BuildKit-фронтенда, а директива тянет отдельный образ
# docker/dockerfile:1. На части окружений он давал выходной образ с нерабочим
# entrypoint'ом («exec format error» при старте контейнера) — лишняя точка
# отказа удалена, функционально сборка не меняется.
FROM node:20-alpine AS build
WORKDIR /app
COPY package.json ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
