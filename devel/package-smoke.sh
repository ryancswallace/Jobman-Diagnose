#!/bin/sh

# Install one amd64 package of each native format in its target distribution
# and verify the executable and packaged documentation.
set -eu

dist=${1:-dist}
manifest=$(find "${dist}" -maxdepth 1 -type f -name 'jobman-diagnose_*_checksums.txt' -print)
if [ -z "${manifest}" ] || [ "$(printf '%s\n' "${manifest}" | wc -l | tr -d ' ')" -ne 1 ]; then
  echo "expected exactly one Jobman Diagnose checksum manifest in ${dist}" >&2
  exit 1
fi
version=$(basename "${manifest}")
version=${version#jobman-diagnose_}
version=${version%_checksums.txt}
if ! printf '%s\n' "${version}" | grep -Eq '^[0-9][0-9A-Za-z.+-]*$'; then
  echo "invalid package version ${version}" >&2
  exit 1
fi
dist_absolute=$(cd "${dist}" && pwd -P)

alpine='alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b'
debian='debian:bookworm-slim@sha256:abd67ffcfa541b485a3dff59865ab629aa048a6c613e639d36e7456b0b229241'
fedora='fedora:42@sha256:99e203b80b1c3d8f7e161ec10a68fd02b081ef83a3963553e513c82846b97814'

docker run --rm --platform linux/amd64 -v "${dist_absolute}:/dist:ro" "${debian}" \
  sh -eu -c '
    dpkg -i "$1" >/dev/null
    jobman-diagnose --version | grep -F "jobman-diagnose $2"
    dpkg-query -L jobman-diagnose | grep -Fx /usr/share/doc/jobman-diagnose/INSTALLATION.md
    dpkg-query -L jobman-diagnose | grep -Fx /usr/share/licenses/jobman-diagnose/THIRD_PARTY_NOTICES.md
  ' sh "/dist/jobman-diagnose_${version}_linux_amd64.deb" "${version}"

docker run --rm --platform linux/amd64 -v "${dist_absolute}:/dist:ro" "${fedora}" \
  sh -eu -c '
    rpm -i "$1"
    jobman-diagnose --version | grep -F "jobman-diagnose $2"
    test -s /usr/share/doc/jobman-diagnose/INSTALLATION.md
    test -s /usr/share/licenses/jobman-diagnose/THIRD_PARTY_NOTICES.md
  ' sh "/dist/jobman-diagnose_${version}_linux_amd64.rpm" "${version}"

docker run --rm --platform linux/amd64 -v "${dist_absolute}:/dist:ro" "${alpine}" \
  sh -eu -c '
    apk add --no-cache --allow-untrusted "$1" >/dev/null
    jobman-diagnose --version | grep -F "jobman-diagnose $2"
    test -s /usr/share/doc/jobman-diagnose/INSTALLATION.md
    test -s /usr/share/licenses/jobman-diagnose/THIRD_PARTY_NOTICES.md
  ' sh "/dist/jobman-diagnose_${version}_linux_amd64.apk" "${version}"

printf 'Installed and verified Jobman Diagnose %s DEB, RPM, and APK packages.\n' "${version}"
