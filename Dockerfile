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

# Add cron job to pull the repo every minute
RUN echo "* * * * * cd /app/ert_wiki && git pull "git@github.com:EPFLRocketTeam/ert_wiki.git" > /var/log/cron.log 2>&1" > /etc/cron.d/ert_wiki_cron \
    && chmod 0644 /etc/cron.d/ert_wiki_cron \
    && crontab /etc/cron.d/ert_wiki_cron
    
# Copy application code
COPY ./app .
COPY gunicorn.conf.py .
COPY ImageLuaFilter.lua .
COPY .ssh /root/.ssh:ro
COPY --chmod=0600 .ssh/id_ed25519 /root/.ssh/id_ed25519
COPY --chmod=0600 .ssh/id_rsa /root/.ssh/id_rsa
    
# Initialise ssh keys to pull updates from repo
RUN cd /root/.ssh \
    && eval "$(ssh-agent -s)" \
    && ssh-add id_ed25519 \
    && ssh-add id_rsa \
    && ssh-keyscan github.com >> /root/.ssh/known_hosts \
    && chmod 644 /root/.ssh/known_hosts \
    && cd /app/ert_wiki \
    && git config --global --add safe.directory /app/ert_wiki \
    && git pull
# Ensure cron runs in the container
CMD ["sh", "-c", "cron && gunicorn --config gunicorn.conf.py app:app"]

# Expose port 8000 (Gunicorn default)
EXPOSE 8000
