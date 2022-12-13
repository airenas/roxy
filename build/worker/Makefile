-include ../../version 
#####################################################################################
service=airenas/roxy-worker
version?=$(worker_version)
########### DOCKER ##################################################################
tag=$(service):$(version)

dbuild:
	cd ../../ && cat build/worker/Dockerfile | docker buildx build -t $(tag) --build-arg BUILD_VERSION=$(version) -f - .

dpush: dbuild
	docker push $(tag)

dscan: dbuild
	docker scan --accept-license $(tag)	
#####################################################################################
.PHONY: dbuild dpush dscan
