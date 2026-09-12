FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/sshire ./cmd/sshire

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/sshire /sshire
EXPOSE 22 8080
VOLUME ["/data"]
ENTRYPOINT ["/sshire", "-config", "/data/config.yaml"]
