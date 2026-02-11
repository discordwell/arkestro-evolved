FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/arkessro ./cmd/arkessro

FROM alpine:3.21

RUN adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /out/arkessro /app/arkessro
RUN mkdir -p /data && chown -R app:app /data /app

USER app

EXPOSE 8080

ENTRYPOINT ["/app/arkessro"]
CMD ["-addr", "0.0.0.0:8080", "-db", "/data/dev.db"]
