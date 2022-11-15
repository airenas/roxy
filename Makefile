sql_dir=sql
#####################################################################################
## print usage information
help:
	@echo 'Usage:'
	@cat ${MAKEFILE_LIST} | grep -e "^## " -A 1 | grep -v '\-\-' | sed 's/^##//' | cut -f1 -d":" | \
		awk '{info=$$0; getline; print "  " $$0 ": " info;}' | column -t -s ':' 
.PHONY: help
#####################################################################################
migrate/install: 
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
.PHONY: migrate/install
migrate/new: 
	migrate create -ext sql -dir $(sql_dir) -seq $(sql_name)
.PHONY: migrate/new
#####################################################################################
## build docker for provided service
docker/%/build: 
	cd build/$* && $(MAKE) dbuild
.PHONY: docker/*/build
#####################################################################################
## build all containers
build/all: docker/dbmigrate/build
.PHONY: build/all
#####################################################################################
## push docker for provided service
docker/%/push: 
	cd build/$* && $(MAKE) dpush
.PHONY: docker/*/push
#####################################################################################
## scan docker for provided service
docker/%/scan: 
	cd build/$* && $(MAKE) dscan
.PHONY: docker/*/scan
