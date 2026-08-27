# syntax=docker/dockerfile:1
FROM golang:1.27 AS build
WORKDIR /src

COPY go.mod ./
COPY *.go ./

# Cache mounts persist Go's module and build caches across builds (BuildKit-only,
# not baked into the image), so repeated builds only recompile changed files.
# -ldflags="-s -w" strips the symbol table (-s) and DWARF debug info (-w),
# shrinking the binary since we don't need debugger/profiler support in the image.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/randomfail .

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /out/randomfail /randomfail

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/randomfail"]
