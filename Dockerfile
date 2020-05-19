# Build the manager binary
FROM golang:1.13.5 as builder

# Copy in the go src
WORKDIR /go/src/github.com/ibm/cloud-operators
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager github.com/ibm/cloud-operators/cmd/manager

# Copy the controller-manager into a thin image
FROM registry.access.redhat.com/ubi8-minimal
WORKDIR /root/
COPY --from=builder /go/src/github.com/ibm/cloud-operators/manager .
COPY git-rev .
ENTRYPOINT ["./manager"]
