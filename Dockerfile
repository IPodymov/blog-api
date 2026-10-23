FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /out/api ./cmd/api && CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /out/migrate ./cmd/migrate && CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /out/maintenance ./cmd/maintenance
FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/ /app/
USER app
EXPOSE 8080
CMD ["/app/api"]
