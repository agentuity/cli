FROM ubuntu:22.04

# Install dependencies
RUN apt-get update && \
    apt-get install -y curl tar ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Copy install.sh into the image
COPY install.sh ./

# Make it executable
RUN chmod +x install.sh

# Add a script to run the install and verification steps
COPY install_test/docker-test-install.sh ./
RUN chmod +x docker-test-install.sh

CMD ["./docker-test-install.sh"] 