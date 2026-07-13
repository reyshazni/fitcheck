FROM golang:1.26 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
      -X github.com/reyshazni/fitcheck/internal/version.buildVersion=${VERSION} \
      -X github.com/reyshazni/fitcheck/internal/version.buildCommit=${COMMIT} \
      -X github.com/reyshazni/fitcheck/internal/version.buildDate=${DATE}" \
    -o fitcheck ./cmd/

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /workspace/fitcheck /fitcheck

USER 65532:65532

ENTRYPOINT ["/fitcheck"]
