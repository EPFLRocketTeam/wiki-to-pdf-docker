FROM python:3.9-slim

# Set working directory
WORKDIR /app

# Ensure the mountpoint exists in the image so runtime bind mounts have a directory
RUN mkdir -p /app/ert_wiki && chown root:root /app/ert_wiki

# Install system dependencies
RUN apt-get update && apt-get install -y \
    texlive-latex-recommended \
    texlive-fonts-recommended \
    texlive-science \
    texlive-latex-extra \
    texlive-bibtex-extra \
    texlive-luatex \
    texlive-pictures \
    texlive-plain-generic \
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
    ca-certificates \
    libgbm1 \
    libpangocairo-1.0-0 \
    libcurl4 \
    inkscape \
    cron \
    git \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
    
# Copy application code
COPY ./app .
COPY gunicorn.conf.py .
COPY ImageLuaFilter.lua .

# Ensure the bind-mounted wiki directory can be accessed safely by git if needed.
RUN git config --global --add safe.directory /app/ert_wiki

CMD ["gunicorn", "--config", "gunicorn.conf.py", "app:app"]

# Expose port 8000 (Gunicorn default)
EXPOSE 8000
