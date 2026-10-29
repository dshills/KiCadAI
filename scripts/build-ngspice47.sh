#!/usr/bin/env bash
# Explicit dependency preparation only: never installs or links system software.
set -euo pipefail
if [[ $# != 1 || -z "$1" ]]; then
  echo 'Usage: bash scripts/build-ngspice47.sh NEW_CACHE_DIRECTORY' >&2
  exit 2
fi
destination=$1
if [[ -e "$destination" || -L "$destination" ]]; then
  echo 'Refusing to overwrite an existing cache directory' >&2
  exit 2
fi
mkdir -p "$destination"
destination=$(cd "$destination" && pwd -P)
cd "$destination"
curl --fail --location --proto '=https' --proto-redir '=https' --tlsv1.2 --max-time 90 --output source.tar.gz \
  https://downloads.sourceforge.net/project/ngspice/ng-spice-rework/47/ngspice-47.tar.gz
expected=894e649651f1838a14095e5a5439e7d3aa63e87ede14d283173fda4fcdef675f
actual=$(shasum -a 256 source.tar.gz)
if [[ ${actual%% *} != "$expected" ]]; then
  echo 'ngspice 47 source digest mismatch; refusing to extract or build' >&2
  exit 1
fi
tar -xzf source.tar.gz
cd ngspice-47
./configure --prefix="$destination/install" --without-x --with-readline=no --disable-openmp --disable-debug
make -j4
make install
"$destination/install/bin/ngspice" -n --version
