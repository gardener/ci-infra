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
    source=$(yq '.source' <<< $image)
    destination=$(yq '.destination' <<< $image)
    # Loop over the tags for the current image
    for tag in $(yq '.tags[] | @json' <<< $image ); do
        # Copy the container image for the current tag using crane
        source_tag=$(yq '.source'<<< $tag)
        destination_tag=$(yq '.destination // .source'<<< $tag)
        crane copy "$source:$source_tag" "$destination:$destination_tag"
    done
done
