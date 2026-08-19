FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY packages ./packages
COPY apps/inventory ./apps/inventory
RUN CGO_ENABLED=0 go build -o /out/server ./apps/inventory/cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/server /app/server
EXPOSE 8102
ENV COMMERCE_HTTP_PORT=8102
CMD ["/app/server"]
