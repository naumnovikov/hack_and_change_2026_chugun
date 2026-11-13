FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go *.txt ./
COPY testdata ./testdata
RUN CGO_ENABLED=0 go build -trimpath -o /backend .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /backend ./backend
COPY GO_FONT_LICENSE.txt FPDF_LICENSE.txt X_IMAGE_LICENSE.txt ./
USER app
EXPOSE 8080
ENTRYPOINT ["/app/backend"]
