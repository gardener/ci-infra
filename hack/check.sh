#!/bin/bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -e

GOLANGCI_LINT_CONFIG_FILE=""

for arg in "$@"; do
  case $arg in
    --golangci-lint-config=*)
    GOLANGCI_LINT_CONFIG_FILE="-c ${arg#*=}"
    shift
    ;;
  esac
done

echo "> Check"

echo "Executing golangci-lint"
golangci-lint run $GOLANGCI_LINT_CONFIG_FILE --timeout 10m $@

if [ -d "./vendor" ]; then
  VET_MOD_OPTS=-mod=vendor
else
  VET_MOD_OPTS=-mod=readonly
fi

echo "Executing go vet"
go vet ${VET_MOD_OPTS} $@

echo "Executing gofmt/goimports"
folders=()
for f in $@; do
  folders+=( "$(echo $f | sed 's/\.\.\.//')" )
done
unformatted_files="$(goimports -l ${folders[*]})"
if [[ "$unformatted_files" ]]; then
  echo "Unformatted files detected:"
  echo "$unformatted_files"
  exit 1
fi

echo "All checks successful"
