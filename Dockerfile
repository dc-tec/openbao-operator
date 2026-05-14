# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.26.3@sha256:633d23bf362cb40dd72b4f277288a8929697d77537f9c801b81aeced19b5bdf3 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG SOURCE_DATE_EPOCH=0
ENV SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH}

WORKDIR /workspace
# Copy Go module and vendored dependency manifests.
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/
RUN test -f vendor/modules.txt

# Copy the Go source (relies on .dockerignore to filter)
COPY . .

# Build
# the GOARCH has no default value to allow the binary to be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
  go build -a -mod=vendor -trimpath -buildvcs=false -ldflags="-buildid=" -o manager cmd/main.go && \
  touch -h -d "@${SOURCE_DATE_EPOCH}" manager

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:e3f945647ffb95b5839c07038d64f9811adf17308b9121d8a2b87b6a22a80a39
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

# Disable Docker-native healthchecks as we rely on Kubernetes Probes (Manager)
HEALTHCHECK NONE

ENTRYPOINT ["/manager"]
