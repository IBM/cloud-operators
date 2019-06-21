# Build the manager binary
FROM golang:1.10.3 as builder

# Copy in the go src
WORKDIR /go/src/github.com/ibm/cloud-operators
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager github.com/ibm/cloud-operators/cmd/manager

# Copy the controller-manager into a thin image
FROM ubuntu:latest
RUN apt-get update && apt-get dist-upgrade -y && apt-get install --no-install-recommends -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /root/
COPY --from=builder /go/src/github.com/ibm/cloud-operators/manager .
COPY git-rev .
ENTRYPOINT ["./manager"]
