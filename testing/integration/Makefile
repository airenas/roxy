-include .env
-include ../../version
#####################################################################################
async?=-d
#####################################################################################
## print usage information
help:
	@echo 'Usage:'
	@cat ${MAKEFILE_LIST} | grep -e "^## " -A 1 | grep -v '\-\-' | sed 's/^##//' | cut -f1 -d":" | \
		awk '{info=$$0; getline; print "  " $$0 ": " info;}' | column -t -s ':' 
.PHONY: help
#####################################################################################
## start all containers
start: 
	docker compose up $(async) db migrate minio upload
.PHONY: start
psql/roxy:
	docker compose exec -e PGPASSWORD=$(PSQL_PASS) db psql -U roxy -h localhost -d asr
.PHONY: psql/roxy
psql/postgres:
	docker compose exec -e PGPASSWORD=$(PSQL_PASS) db psql -U postgres -h localhost -d asr
.PHONY: psql/postgres
# invoke integration tests
test/integration: start 
	docker compose up --build --exit-code-from integration-tests integration-tests
.PHONY: test/integration
# invoke integration tests in verbose mode
test/integration/tmp:  
	docker compose up --build --exit-code-from integration-tests 
.PHONY: test/integration/tmp
## clean everything, stops docker containers and removes them
clean:
	docker compose down 
.PHONY: clean	
#####################################################################################
.EXPORT_ALL_VARIABLES:
