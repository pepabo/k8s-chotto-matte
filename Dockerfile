FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags "-X main.version=$VERSION" -o /out/k8s-chotto-matte .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/k8s-chotto-matte /k8s-chotto-matte
ENTRYPOINT ["/k8s-chotto-matte"]
