FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY packages ./packages
COPY apps/payment ./apps/payment
RUN CGO_ENABLED=0 go build -o /out/server ./apps/payment/cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/server /app/server
EXPOSE 8104
ENV COMMERCE_HTTP_PORT=8104
CMD ["/app/server"]
