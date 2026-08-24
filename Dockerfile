FROM golang:1.25-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o bin/api ./cmd/api

FROM alpine
COPY --from=build /app/bin/api /api
CMD ["/api"]
