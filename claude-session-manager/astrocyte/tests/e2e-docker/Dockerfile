FROM ubuntu:22.04

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies
RUN apt-get update && \
    apt-get install -y \
        python3.11 \
        python3-pip \
        tmux \
        git \
        curl \
        ca-certificates \
        wget \
        procps \
        vim \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create symlinks for python
RUN ln -sf /usr/bin/python3.11 /usr/bin/python3 && \
    ln -sf /usr/bin/python3.11 /usr/bin/python

# Install Python dependencies for astrocyte
RUN pip3 install --no-cache-dir \
    pyyaml>=6.0 \
    requests>=2.31.0

# Create test user (non-root for realistic testing)
RUN useradd -m -s /bin/bash testuser && \
    mkdir -p /home/testuser/src/sessions && \
    mkdir -p /home/testuser/.csm/astrocyte/{diagnoses,prompts,logs} && \
    chown -R testuser:testuser /home/testuser

USER testuser
WORKDIR /home/testuser

# Setup directory structure for CSM sessions
RUN mkdir -p src/sessions

# Copy astrocyte source code
COPY --chown=testuser:testuser astrocyte/astrocyte.py /home/testuser/astrocyte/
COPY --chown=testuser:testuser astrocyte/requirements.txt /home/testuser/astrocyte/

# Copy test scripts and fixtures
COPY --chown=testuser:testuser astrocyte/tests/e2e-docker/scripts/ /tests/scripts/
COPY --chown=testuser:testuser astrocyte/tests/e2e-docker/fixtures/ /tests/fixtures/
COPY --chown=testuser:testuser astrocyte/tests/e2e-docker/tests/ /tests/

# Make test scripts executable
USER root
RUN chmod +x /tests/scripts/*.sh /tests/*.sh
USER testuser

# Create mock CSM commands for testing
RUN mkdir -p /home/testuser/bin
COPY --chown=testuser:testuser astrocyte/tests/e2e-docker/mocks/ /home/testuser/bin/
RUN chmod +x /home/testuser/bin/*

# Add mock CSM to PATH
ENV PATH="/home/testuser/bin:${PATH}"

# Create tmux socket directory
RUN mkdir -p /tmp

# Set working directory to tests
WORKDIR /tests

# Default command: Run full test suite
CMD ["/bin/bash", "/tests/run-all-tests.sh"]
