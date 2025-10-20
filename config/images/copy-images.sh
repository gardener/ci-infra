#!/bin/sh

set -o errexit
set -o nounset

filepath="$1"
if [ -z "$filepath" ] ; then
  echo "Please specify a path to the images.yaml configuration file as the first argument."
  exit 1
fi

# Track if any errors occurred
errors=0

# Loop over the images in the YAML
for image in $(yq '.images[] | @json' "$filepath"); do
    source=$(echo $image | yq '.source')
    destination=$(echo $image | yq '.destination')
    # Loop over the tags for the current image
    for tag in $(echo $image | yq '.tags[]' ); do
        # Copy the container image for the current tag using crane
        # Temporarily disable errexit for the crane copy command
        set +o errexit
        crane copy "$source:$tag" "$destination:$tag"
        if [ $? -ne 0 ]; then
            echo "Error copying $source:$tag to $destination:$tag"
            errors=$((errors + 1))
        fi
        # Re-enable errexit
        set -o errexit
    done
done

# Exit with error if any copy operation failed
if [ $errors -gt 0 ]; then
    echo "Failed to copy $errors image(s)"
    exit 1
fi

echo "All images copied successfully"
