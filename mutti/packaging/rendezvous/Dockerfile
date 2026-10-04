# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS build
ARG TARGETARCH
WORKDIR /src
COPY mutti/connect/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /mutti-connect ./cmd/mutti-connect
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /mutti-connect /mutti-connect
USER 65532:65532
EXPOSE 18600/tcp 3478/udp
ENTRYPOINT ["/mutti-connect", "--mode", "broker", "--listen", "0.0.0.0:18600", "--stun-listen", "0.0.0.0:3478"]
