# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src

# Copy module manifest first so the dependency layer is cached separately
# from the source. This makes ~80% of subsequent rebuilds a no-op for deps.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO disabled for a tiny static binary; ldflags strip the symbol table.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/tinylink \
    ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tinylink /tinylink
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/tinylink"]
