FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN go build -tags plc4x -o /out/connector .

FROM alpine:3.20
COPY --from=build /out/connector /connector
EXPOSE 9001
ENTRYPOINT ["/connector"]
