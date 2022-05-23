all: clean build start

prod: clean build start-prod

build: go/bin/opintegrations

go/bin/opintegrations:
	@echo "Building go/bin/opintegrations"
	@go install ./opintegrations

start:
	@echo "Starting the server"
	@ENV="local" $(GOPATH)/bin/opintegrations

start-prod:
	@echo "Starting prod server"
	@ENV="prod" $(GOPATH)/bin/opintegrations &

stop:
	@-killall opintegrations

clean:
	@echo "Cleaning builds"
	@rm -f $(GOPATH)/bin/opintegrations

.PHONY: all build start clean install go
