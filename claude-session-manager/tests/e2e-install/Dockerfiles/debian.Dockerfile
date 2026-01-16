FROM debian:12

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

# Install Go 1.24 manually (Debian 12 ships with Go 1.19.8)
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
# Need both claude-session-manager and engram/core due to go.mod replace directive
COPY --chown=testuser:testuser ai-tools/main/claude-session-manager /home/testuser/ai-tools/main/claude-session-manager/
COPY --chown=testuser:testuser engram/main/core /home/testuser/engram/main/core/
WORKDIR /home/testuser/ai-tools/main/claude-session-manager

# Build csm from local source
RUN go build -o /home/testuser/go/bin/csm ./cmd/csm

# Copy test scripts
WORKDIR /home/testuser
COPY --chown=testuser:testuser ai-tools/main/claude-session-manager/tests/e2e-install/scripts /tmp/tests/

# Default command: Run full test suite
CMD ["/bin/bash", "/tmp/tests/test-install.sh"]
