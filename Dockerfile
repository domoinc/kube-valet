FROM golang:1.12 as builder

WORKDIR /src/kube-valet

# Test and Build
COPY . .
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -i --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo
RUN cp kube-valet /kube-valet

RUN CGO_ENABLED=0 GOOS=linux go build -i --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo  bin/valetctl.go
RUN cp valetctl /valetctl

## Final image build
FROM alpine:3.2

COPY --from=builder /kube-valet /

ENTRYPOINT ["/kube-valet"]
