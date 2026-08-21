# Build stage
FROM golang:1.24-bookworm AS builder

WORKDIR /opt
COPY . .
RUN go mod tidy
RUN go build -o ofc-user-service

# Run stage
FROM golang:1.24-bookworm
WORKDIR /opt

COPY --from=builder /opt/ofc-user-service .
COPY conf/config.json /opt/conf/config.json
COPY seeds /opt/seeds

CMD ["/opt/ofc-user-service"]
