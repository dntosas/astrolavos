FROM golang:1.17-alpine as builder

RUN apk add git make

# Golang directories structure
RUN mkdir -p /astrolavos
ADD . /astrolavos
WORKDIR /astrolavos

RUN make build

# Shrink final image a bit
FROM alpine:3.12
RUN apk update && apk add ca-certificates

# We only need html templates and the binary, we have a TODO to package all
# htmls to the binary later
COPY --from=builder /astrolavos/bin/astrolavos /astrolavos/astrolavos

WORKDIR /astrolavos
ENTRYPOINT ["./astrolavos"]
