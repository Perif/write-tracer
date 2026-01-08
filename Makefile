# Makefile
.PHONY: generate build clean run caps install

BIN := write-tracer
CMD := ./cmd/tracer

generate:
	go generate ./...

build: generate
	go build -o $(BIN) $(CMD)

clean:
	rm -f $(BIN)
	rm -f internal/ebpf/bpf_*.go internal/ebpf/bpf_*.o

caps: build
	sudo setcap 'cap_bpf+ep cap_perfmon+ep cap_dac_override+ep' ./$(BIN)
	@getcap ./$(BIN)

run: build
	sudo ./$(BIN) $(ARGS)

deps:
	go mod tidy
	go mod download

all: clean deps build

install: build
	sudo cp $(BIN) /usr/local/bin/
	sudo setcap 'cap_bpf+ep cap_perfmon+ep cap_dac_override+ep' /usr/local/bin/$(BIN)