FROM golang:1.21.3-alpine3.18 as builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 go build -o http_dl ./main.go


FROM alpine:3.18

WORKDIR /app
COPY --from=builder /app/http_dl .

ENTRYPOINT [ "sh" ]