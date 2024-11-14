FROM python:3.9-slim
RUN pip install --no-cache-dir -r requirements.txt

FROM ubuntu:24.04
RUN \
  apt update && \
  apt-get -y install texlive-full python3 \
  && pip install --no-cache-dir -r requirements.txt

WORKDIR /app

# Install dependencies
COPY requirements.txt .

# Copy application code
COPY ./app .
COPY gunicorn.conf.py .

# Expose port 8000 (Gunicorn's default)
EXPOSE 8000

# Run the application with Gunicorn
CMD ["gunicorn", "--config", "gunicorn.conf.py", "app:app"]
