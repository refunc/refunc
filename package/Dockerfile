FROM alpine:3.7

LABEL maintainer="antmanler(wo@zhaob.in)"

RUN apk --no-cache add ca-certificates bash wget curl

VOLUME [ "/var/run/refunc" ]

ARG BIN_TARGET
ENV CMD_TARGET=${BIN_TARGET}

RUN echo building for ${BIN_TARGET}

COPY $BIN_TARGET /usr/bin/
CMD exec "$CMD_TARGET"
