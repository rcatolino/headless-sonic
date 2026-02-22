sources := $(wildcard pkg/**/*.go) $(wildcard cmd/**/*.go)
bin := headless-sonic

.PHONY: build clean
.SUFFIXES:

build: $(bin) $(bin)-arm

$(bin)-arm: $(sources) go.mod go.sum
	GOOS=linux GOARCH=arm GOARM=6 go build -o $(bin)-arm ./cmd/app.go

$(bin): $(sources) go.mod go.sum
	go build -o $(bin) ./cmd/app.go

clean:
	-rm $(bin)
	-rm $(bin)-arm
