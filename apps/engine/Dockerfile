# InsightIQ investigation engine (Go)
# Railway/Fly inject PORT at runtime — do not pin ENGINE_PORT here.
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/engine .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/engine /app/engine
EXPOSE 8080
USER nobody
CMD ["/app/engine"]
