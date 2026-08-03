set dotenv-load := true
set shell := ["bash", "-euo", "pipefail", "-c"]

app := "kinops"
main := "./cmd/kinops"
bin := "./bin/" + app

default:
    @just --list

bootstrap:
    go mod download
    go install github.com/a-h/templ/cmd/templ@latest
    go install github.com/air-verse/air@latest

generate:
    templ generate

fmt:
    templ fmt .
    gofmt -w .
    go mod tidy

build: generate
    mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -o {{bin}} {{main}}

run: generate
    mkdir -p data
    go run {{main}}

admin-password-hash:
    go run {{main}} hash-password

dev:
    mkdir -p data
    templ generate \
        --watch \
        --proxy="http://localhost:8081" \
        --cmd="go run {{main}}"

test:
    go test -race ./...

mealie-smoke:
    KINOPS_MEALIE_SMOKE=1 MEALIE_BASE_URL=http://127.0.0.1:9925 go test -v -run TestLiveMealieReadContract ./internal/mealie

check: generate
    go vet ./...
    go test -race ./...

clean:
    rm -rf bin tmp coverage

compose-build:
    docker compose build

compose-up:
    docker compose up --build

compose-down:
    docker compose down

compose-logs:
    docker compose logs -f kinops

compose-reset:
    docker compose down --volumes
