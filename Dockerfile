FROM golang:1.26.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/the-source .
RUN mkdir -p /out/state

FROM scratch
COPY --from=build /out/the-source /the-source
COPY source.docker.toml /etc/source.toml
COPY --from=build /out/state/ /var/lib/source/
USER 0:0
EXPOSE 45068
ENTRYPOINT ["/the-source"]
CMD ["-config", "/etc/source.toml"]
