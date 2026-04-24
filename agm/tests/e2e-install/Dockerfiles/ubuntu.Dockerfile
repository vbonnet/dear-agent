FROM ubuntu:24.04 AS base

# Prevent interactive prompts during installation
ENV DEBIAN_FRONTEND=noninteractive

# Install dependencies required by AGM
RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    tmux \
    git \
    curl \
    make \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.25
RUN curl -fsSL https://go.dev/dl/go1.25.0.linux-amd64.tar.gz -o /tmp/go.tar.gz \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz

ENV PATH="/usr/local/go/bin:/go/bin:${PATH}"
ENV GOPATH="/go"

# Setup test user (non-root installation)
RUN useradd -m -s /bin/bash testuser \
    && mkdir -p /go/bin \
    && chown -R testuser:testuser /go

# --- Build stage: cache Go module downloads ---
FROM base AS build

COPY --chown=testuser:testuser . /home/testuser/ai-tools

USER testuser
WORKDIR /home/testuser/ai-tools

# Download modules first (cached layer)
RUN go mod download

# Build and install AGM binaries
RUN go build -o /go/bin/agm ./agm/cmd/agm && \
    go build -o /go/bin/agm-reaper ./agm/cmd/agm-reaper && \
    go build -o /go/bin/agm-mcp-server ./agm/cmd/agm-mcp-server

# --- Final stage: minimal runtime ---
FROM base AS final

USER testuser
WORKDIR /home/testuser

# Copy built binaries from build stage
COPY --from=build /go/bin/agm /go/bin/agm
COPY --from=build /go/bin/agm-reaper /go/bin/agm-reaper
COPY --from=build /go/bin/agm-mcp-server /go/bin/agm-mcp-server

# Verify installation
RUN agm --help && \
    agm-reaper --help && \
    echo "AGM installation verified on Ubuntu 24.04"

# Cleanup
RUN rm -rf /tmp/* ~/.cache/go-build

ENTRYPOINT ["/bin/bash", "-c", "echo 'AGM E2E install test passed (Ubuntu)' && agm --help"]
