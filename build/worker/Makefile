-include ../../version 
#####################################################################################
service=airenas/roxy-worker
version?=$(worker_version)
file=build/worker/Dockerfile
########### DOCKER ##################################################################
tag=$(service):$(version)

dbuild:
	docker buildx build -t $(tag) --build-arg BUILD_VERSION=$(version) -f Dockerfile ./../..

dpush: dbuild
	docker push $(tag)

dscan: dbuild
	docker scan --accept-license $(tag)	
#####################################################################################
.PHONY: dbuild dpush dscan
