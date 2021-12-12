FROM golang:latest AS builder

ARG VERSION=HEAD
ARG COMMIT=UNKNOWN

ENV CGO_ENABLED 0

RUN mkdir /output

COPY . /app
RUN cd /app && go build --trimpath -ldflags="-s -w -X 'main.version=$VERSION' -X 'main.commit=$COMMIT'" -o /output/artistore

RUN apt update && apt install -y upx && upx --lzma /output/*


FROM scratch

COPY --from=builder /output /usr/bin

EXPOSE 3000
VOLUME /var/lib/artistore

ENTRYPOINT ["/usr/bin/artistore"]
