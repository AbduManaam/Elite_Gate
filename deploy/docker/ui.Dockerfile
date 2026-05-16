FROM node:20-alpine AS builder
WORKDIR /app
COPY web/admin-ui/package*.json ./
RUN npm ci
COPY web/admin-ui/ .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80