-include ../../version 
#####################################################################################
service=airenas/roxy-result
version?=$(result_version)
########### DOCKER ##################################################################
tag=$(service):$(version)

dbuild:
	cd ../../ && cat build/result/Dockerfile | docker buildx build -t $(tag) --build-arg BUILD_VERSION=$(version) -f - .

dpush: dbuild
	docker push $(tag)

dscan: dbuild
	docker scan --accept-license $(tag)	
#####################################################################################
.PHONY: dbuild dpush dscan
