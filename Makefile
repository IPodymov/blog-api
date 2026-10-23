.PHONY: run migrate test integration check openapi build
run:
	@mkdir -p bin
	go build -o bin/api ./cmd/api
	@exec ./bin/api
migrate:
	go run ./cmd/migrate
test:
	go test -race ./...
integration:
	@test -n "$(TEST_DATABASE_URL)" || (echo 'Set TEST_DATABASE_URL to a dedicated PostgreSQL database'; exit 1)
	go test -race -count=1 -tags=integration ./...
openapi:
	npm run check:openapi
check: openapi
	@test -z "$$(gofmt -l api cmd internal migrations)" || (gofmt -l api cmd internal migrations; exit 1)
	go vet ./...
	go test -race ./...
build:
	go build ./cmd/api ./cmd/migrate ./cmd/maintenance

maintenance:
	go run ./cmd/maintenance
