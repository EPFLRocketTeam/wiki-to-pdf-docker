FROM python:3.9-slim

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    texlive-latex-recommended \
    texlive-fonts-recommended \
    texlive-fonts-extra \
    texlive-science \
    texlive-latex-extra \
    texlive-bibtex-extra \
    texlive-full \
    python3 \
    binutils \
    plantuml \
    wget \
    curl \
    xvfb \
    xauth \
    libnss3 \
    libxss1 \
    libasound2 \
    libatk1.0-0 \
    libgtk-3-0 \
    libx11-xcb1 \
    libnotify4 \
    libxtst6 \
    libsecret-1-0 \
    libappindicator3-1 \
    fonts-liberation \
    libgconf-2-4 \
    ca-certificates \
    libgbm1 \
    libpangocairo-1.0-0 \
    libcurl4 \
    inkscape \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Draw.io (CLI version)
RUN curl -s https://api.github.com/repos/jgraph/drawio-desktop/releases/latest \
    | grep browser_download_url \
    | grep '\.deb' \
    | cut -d '"' -f 4 \
    | wget -i - \
    && apt install -y ./drawio-amd64-*.deb \
    && rm drawio-amd64-*.deb

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY ./app .
COPY gunicorn.conf.py .
COPY ./ert_wiki ./ert_wiki
COPY ImageLuaFilter.lua .

# Expose port 8000 (Gunicorn default)
EXPOSE 8000

# Default command (run the app)
CMD ["gunicorn", "--config", "gunicorn.conf.py", "app:app"]
