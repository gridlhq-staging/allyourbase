FROM node:20-alpine AS ui-builder

WORKDIR /src/ui
COPY ui/package.json ui/pnpm-lock.yaml ./
RUN npm install --legacy-peer-deps
COPY ui/ .
RUN npm run build

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /src/ui/dist ./ui/dist

RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /ayb ./cmd/ayb

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /ayb /usr/local/bin/ayb

EXPOSE 8090

ENTRYPOINT ["ayb"]
CMD ["start"]
