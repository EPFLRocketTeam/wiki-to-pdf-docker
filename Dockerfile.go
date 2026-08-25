FROM golang:1.22-bookworm AS builder

WORKDIR /src
COPY go-service/go.mod go-service/go.sum ./
RUN go mod download

COPY go-service ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/wiki-to-pdf-go ./cmd/server

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    wget \
    git \
    texlive-latex-recommended \
    texlive-fonts-recommended \
    texlive-fonts-extra \
    texlive-science \
    texlive-latex-extra \
    texlive-bibtex-extra \
    texlive-full \
    plantuml \
    inkscape \
    pandoc \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/wiki-to-pdf-go /app/wiki-to-pdf-go
COPY ImageLuaFilter.lua /app/ImageLuaFilter.lua
COPY app/latex_templates /app/latex_templates

RUN mkdir -p /app/ert_wiki

ENV LISTEN_ADDR=:8000

EXPOSE 8000

ENTRYPOINT ["/app/wiki-to-pdf-go"]
