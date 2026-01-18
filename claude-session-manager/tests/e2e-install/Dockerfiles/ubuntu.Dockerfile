FROM ubuntu:22.04

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install prerequisites
RUN apt-get update && \
    apt-get install -y \
        git \
        curl \
        ca-certificates \
        wget \
        tar && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Install Go 1.24 manually (Ubuntu 22.04 doesn't have golang-1.24 package)
RUN wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz && \
    rm go1.24.0.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:$PATH"

# Create non-root user for realistic testing
RUN useradd -m -s /bin/bash testuser
USER testuser
WORKDIR /home/testuser

# Setup Go environment for testuser
ENV PATH="/home/testuser/go/bin:$PATH"
ENV GOPATH="/home/testuser/go"

# Copy source code for local build (repo is private, can't use go install)
COPY --chown=testuser:testuser claude-session-manager /home/testuser/claude-session-manager/
WORKDIR /home/testuser/claude-session-manager

# Build csm from local source
RUN go build -o /home/testuser/go/bin/csm ./cmd/csm

# Copy test scripts
WORKDIR /home/testuser
COPY --chown=testuser:testuser claude-session-manager/tests/e2e-install/scripts /tmp/tests/

# Default command: Run full test suite
CMD ["/bin/bash", "/tmp/tests/test-install.sh"]
