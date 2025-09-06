#!/bin/bash

# Nome del container
CONTAINER_NAME="ledger-service"

# Function to show usage information
usage() {
  echo "Usage: $0 [-r]"
  echo "  -r    Removes the container after stopping it."
  exit 1
}

# Checks the flags
REMOVE_CONTAINER=false
while getopts "r" opt; do
  case $opt in
    r)
      REMOVE_CONTAINER=true
      ;;
    *)
      usage
      ;;
  esac
done

# Stops the container
echo "Stopping container $CONTAINER_NAME..."
docker stop $CONTAINER_NAME

# Removes the container (if required)
if [ "$REMOVE_CONTAINER" = true ]; then
  echo "Removing container $CONTAINER_NAME..."
  docker rm $CONTAINER_NAME
fi

echo "Operation completed."