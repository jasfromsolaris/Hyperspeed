FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY . .
ARG VERSION=dev
ARG GIT_SHA=
RUN go mod tidy && CGO_ENABLED=0 go build -ldflags "-X hyperspeed/api/internal/version.Version=${VERSION} -X hyperspeed/api/internal/version.GitSHA=${GIT_SHA}" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata git
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
