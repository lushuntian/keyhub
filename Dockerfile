FROM node:24-alpine AS web-build
WORKDIR /app/web
COPY web/package*.json ./
RUN npm install
COPY web ./
RUN npm run build

FROM golang:1.25-alpine AS go-build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/keyhub ./cmd/keyhub

FROM alpine:3.22
WORKDIR /app
RUN adduser -D -H keyhub
COPY --from=go-build /out/keyhub /app/keyhub
COPY migrations /app/migrations
COPY --from=web-build /app/web/dist /app/web/dist
USER keyhub
EXPOSE 8080
CMD ["/app/keyhub"]
