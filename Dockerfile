FROM golang:1.26-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o bin/api ./cmd/api
# Admin bootstrap for freshly migrated databases (specs/database-migrations/ BR10),
# run as a one-off command/Job with the same image: /bootstrap-admin
RUN go build -o bin/bootstrap-admin ./cmd/bootstrap-admin

FROM alpine
COPY --from=build /app/bin/api /api
COPY --from=build /app/bin/bootstrap-admin /bootstrap-admin
CMD ["/api"]
