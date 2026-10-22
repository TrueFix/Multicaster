VERSION ?= $(shell cat VERSION)
TAG ?= v$(VERSION)
REPOSITORY ?= TrueFix/Multicaster

.PHONY: build web test check release run
build: web
	go build -buildvcs=false -trimpath -ldflags="-s -w -X github.com/truefix/multicaster/internal/updates.Version=$(VERSION)" -o dist/multicaster ./cmd/multicaster

web:
	npm --prefix frontend ci
	npm --prefix frontend run build

test:
	go test -race ./...
	go vet ./...

check: test
	npm --prefix frontend run test:unit
	python3 scripts/release.py validate --tag "$(TAG)"
	python3 -m unittest discover -s scripts -p 'test_*.py'
	sh -n scripts/install.sh

release: web
	python3 scripts/release.py build --tag "$(TAG)" --repository "$(REPOSITORY)"

run:
	go run ./cmd/multicaster
