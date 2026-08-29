FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/docmost-openviking-sync ./cmd/docmost-openviking-sync

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/docmost-openviking-sync /usr/local/bin/docmost-openviking-sync
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/docmost-openviking-sync"]
CMD ["daemon"]
