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
	pandoc \
	curl \
	wget \
	git \
	make \
	ncurses-bin \
	texlive-latex-recommended \
	texlive-fonts-recommended \
	texlive-science \
	texlive-latex-extra \
	texlive-bibtex-extra \
	texlive-luatex \
	texlive-pictures \
	texlive-plain-generic \
	plantuml \
	inkscape \
	&& apt-get clean && rm -rf /var/lib/apt/lists/*

RUN for package in \
	graphicx titlesec csquotes enumitem caption subcaption hyperref float xcolor \
	amsmath amssymb amsfonts booktabs adjustbox calc colortbl array etoolbox \
	wrapfig ragged2e textcomp gensymb pdflscape pdfpages tikz geometry lastpage \
	helvet soul datetime2 longtable fancyhdr fancyvrb eso-pic tcolorbox \
	fontawesome5 lineno bookmark xurl; do \
	kpsewhich "${package}.sty" >/dev/null || { echo "missing LaTeX package: ${package}"; exit 1; }; \
	done

COPY --from=builder /out/wiki-to-pdf-go /app/wiki-to-pdf-go
COPY ImageLuaFilter.lua /app/ImageLuaFilter.lua
COPY app/latex_templates /app/latex_templates

RUN mkdir -p /app/ert_wiki

ENV LISTEN_ADDR=:8000

EXPOSE 8000

ENTRYPOINT ["/app/wiki-to-pdf-go"]
