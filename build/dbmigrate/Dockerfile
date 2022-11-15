#########################################################################################
## docker will invoke this file from ../.. dir in order to access code
#########################################################################################
FROM migrate/migrate:4

ARG BUILD_VERSION=0.1

LABEL org.opencontainers.image.version=$BUILD_VERSION \
      org.opencontainers.image.authors="airenass@gmail.com" \
      name="roxy-dbmigrate" \
      maintainer="airenass@gmail.com" \
      vendor="airenass@gmail.com" \
      version=$BUILD_VERSION \
      release=$BUILD_VERSION \
      summary="This image is used to migrate roxy db" \
      description="This image is used to migrate roxy db" 

COPY sql /migrations/
