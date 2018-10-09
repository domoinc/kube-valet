FROM golang:1.9 as builder

WORKDIR /go/src/github.com/domoinc/kube-valet

ENV GLIDE_VERSION v0.13.1

# Install glide
RUN curl -L -o - https://github.com/Masterminds/glide/releases/download/${GLIDE_VERSION}/glide-${GLIDE_VERSION}-linux-amd64.tar.gz | tar --strip-components=1 -C /usr/bin -zxv linux-amd64/glide && chmod +x /usr/bin/glide

# Install deps
COPY glide.yaml glide.lock ./
RUN glide i

# Test and Build
COPY . .
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo
RUN cp kube-valet /kube-valet

## Final image build
FROM alpine:3.2

COPY --from=builder /kube-valet /

ENTRYPOINT ["/kube-valet"]
