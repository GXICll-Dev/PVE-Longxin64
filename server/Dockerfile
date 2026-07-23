# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN build_flags="-s -w -X github.com/GXICll-Dev/PVE-Longxin64/server/internal/buildinfo.Version=$VERSION -X github.com/GXICll-Dev/PVE-Longxin64/server/internal/buildinfo.Commit=$VCS_REF -X github.com/GXICll-Dev/PVE-Longxin64/server/internal/buildinfo.BuildTime=$BUILD_DATE" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="$build_flags" -o /out/pve-classroom-api ./cmd/api && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="$build_flags" -o /out/pve-classroom-worker ./cmd/worker

FROM scratch AS runtime
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/pve-classroom-api /app/pve-classroom-api
COPY --from=build /out/pve-classroom-worker /app/pve-classroom-worker
COPY migrations /app/migrations

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/pve-classroom-api"]
