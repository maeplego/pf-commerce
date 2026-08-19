FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY packages ./packages
COPY apps/order ./apps/order
RUN CGO_ENABLED=0 go build -o /out/server ./apps/order/cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/server /app/server
EXPOSE 8103
ENV COMMERCE_HTTP_PORT=8103
CMD ["/app/server"]
