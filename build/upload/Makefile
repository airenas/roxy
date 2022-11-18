-include ../../version 
#####################################################################################
service=airenas/roxy-upload
version?=$(upload_version)
file=build/upload/Dockerfile
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
