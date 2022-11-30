#########################################################################################
## docker will invoke this file from ../.. dir in order to access code
#########################################################################################
FROM golang:1.19-alpine as builder

ARG BUILD_VERSION=0.1

WORKDIR /go/src/
ENV CGO_ENABLED=0

COPY . /go/src

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -o /go/bin/worker -trimpath -ldflags "-s -w -X main.version=$BUILD_VERSION" cmd/worker/main.go
#####################################################################################
FROM alpine:3.16 as runner

EXPOSE 8000


ENV UID=999
RUN addgroup app && adduser -S -G app app --uid $UID app

WORKDIR /app
RUN mkdir -p /app && chown -R app:app /app
RUN mkdir -p /data && chown -R app:app /data   

ARG BUILD_VERSION=0.1

LABEL org.opencontainers.image.version=$BUILD_VERSION \
      org.opencontainers.image.authors="airenass@gmail.com" \
      name="roxy-worker" \
      maintainer="airenass@gmail.com" \
      vendor="airenass@gmail.com" \
      version=$BUILD_VERSION \
      release=$BUILD_VERSION \
      summary="This image is used to host the ROXY Worker service" \
      description="This image is used to host the ROXY Worker service" 

COPY --from=builder /go/bin/worker /app/
COPY build/worker/config.yaml /app/

RUN chown app:app /app/* /app
 
USER app

ENTRYPOINT ["./worker"]
