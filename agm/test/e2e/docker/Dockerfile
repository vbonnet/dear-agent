FROM ubuntu:24.04

# Prevent interactive prompts during installation
ENV DEBIAN_FRONTEND=noninteractive

# Install dependencies
RUN apt-get update && apt-get install -y \
    tmux \
    git \
    python3 \
    curl \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.25
RUN curl -OL https://go.dev/dl/go1.25.0.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz \
    && rm go1.25.0.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV PATH="${GOPATH}/bin:${PATH}"

# Setup test user
RUN useradd -m -s /bin/bash testuser \
    && mkdir -p /home/testuser/sessions /home/testuser/.config/agm \
    && chown -R testuser:testuser /home/testuser

# Switch to test user
USER testuser
WORKDIR /home/testuser

# Create directories for AGM binaries and test scripts
RUN mkdir -p /home/testuser/bin /home/testuser/tests

# Copy will be done at build time via docker-compose volume mounts
# This ensures we test the actual built binaries

ENTRYPOINT ["/bin/bash"]
