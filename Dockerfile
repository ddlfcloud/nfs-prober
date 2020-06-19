FROM golang:1.13 as builder
WORKDIR /code
ADD go.mod go.sum /code/
RUN go mod download
ADD . .
RUN go build -o /server main.go
FROM gcr.io/distroless/base
WORKDIR /
COPY --from=builder /server /usr/bin/server
ENTRYPOINT [ "server" ]