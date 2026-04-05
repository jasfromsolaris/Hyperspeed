FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /out/control-plane ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/control-plane /control-plane
EXPOSE 8787
ENTRYPOINT ["/control-plane"]
