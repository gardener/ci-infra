#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

filepath="$1"
if [ -z "$filepath" ] ; then
  echo "Please specify a path to the images.yaml configuration file as the first argument."
  exit 1
fi

# Loop over the images in the YAML
for source in $(yq '.images[].source' "$filepath"); do
    image=$(yq '.images[] | select(.source == "'"$source"'")' "$filepath")
    destination=$(echo "$image" | yq '.destination')
    # Loop over the tags for the current image
    for tag in $(echo "$image" | yq '.tags[]'); do
        # Copy the container image for the current tag using crane
        crane copy "$source:$tag" "$destination:$tag"
    done
done
