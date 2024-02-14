# syntax=docker/dockerfile:1
# Build development
FROM golang:1.19
LABEL description="FLEDGE" authors="Maxim De Clercq"
WORKDIR /work
# Download dependencies
COPY go.mod go.sum ./
RUN go mod download
# Expose http port
EXPOSE 80
# Command for healthcheck
HEALTHCHECK CMD curl -f http://localhost:80/ping
# Copy source code
COPY . .
# Run application
CMD ["go", "run", "cmd/fledge"]