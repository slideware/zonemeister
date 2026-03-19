FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 1000 zonemeister

WORKDIR /app
COPY --from=build /app/server .
COPY --from=build /src/templates ./templates
COPY --from=build /src/static ./static

RUN mkdir /data && chown zonemeister:zonemeister /data

EXPOSE 3000
USER zonemeister
ENTRYPOINT ["/app/server"]
