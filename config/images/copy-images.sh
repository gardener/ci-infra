#!/bin/sh

set -o errexit
set -o nounset

filepath="$1"
if [ -z "$filepath" ] ; then
  echo "Please specify a path to the images.yaml configuration file as the first argument."
  exit 1
fi

# Loop over the images in the YAML
for image in $(yq '.images[] | @json' "$filepath"); do
    source=$(echo $image | yq '.source')
    destination=$(echo $image | yq '.destination')
    # Loop over the tags for the current image
    for tag in $(echo $image | yq '.tags[]' ); do
        # Copy the container image for the current tag using crane
        crane copy "$source:$tag" "$destination:$tag"
    done
done
