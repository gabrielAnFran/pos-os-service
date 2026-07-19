FROM golang:1.23-alpine AS builder
ARG TARGET=server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app ./cmd/${TARGET}

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/app /app/app
COPY --from=builder /src/migrations /app/migrations
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/app/app"]
