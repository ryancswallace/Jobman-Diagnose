#!/bin/sh

# Validate the complete archive/native-package release set produced by
# GoReleaser. Snapshot and release workflows both call this helper.
set -eu

dist=${1:-dist}
manifest=$(find "${dist}" -maxdepth 1 -type f \
  -name 'jobman-diagnose_*_checksums.txt' -print)
if [ -z "${manifest}" ] || [ "$(printf '%s\n' "${manifest}" | wc -l | tr -d ' ')" -ne 1 ]; then
  echo "expected exactly one checksum manifest in ${dist}" >&2
  exit 1
fi

manifest_name=$(basename "${manifest}")
version=${manifest_name#jobman-diagnose_}
version=${version%_checksums.txt}
if [ -z "${version}" ]; then
  echo "could not determine release version from ${manifest_name}" >&2
  exit 1
fi

expected=''
for target in \
  darwin_amd64.tar.gz darwin_arm64.tar.gz \
  linux_386.tar.gz linux_amd64.tar.gz linux_arm64.tar.gz \
  windows_386.zip windows_amd64.zip windows_arm64.zip; do
  expected="${expected} jobman-diagnose_${version}_${target}"
done
for architecture in 386 amd64 arm64; do
  for format in apk deb rpm; do
    expected="${expected} jobman-diagnose_${version}_linux_${architecture}.${format}"
  done
done

for filename in ${expected}; do
  if [ ! -s "${dist}/${filename}" ]; then
    echo "release is missing ${filename}" >&2
    exit 1
  fi
  if [ "$(awk -v name="${filename}" '$2 == name { count++ } END { print count + 0 }' \
    "${manifest}")" -ne 1 ]; then
    echo "checksum manifest must contain ${filename} exactly once" >&2
    exit 1
  fi
  if [ ! -s "${dist}/${filename}.sbom.json" ]; then
    echo "release is missing ${filename}.sbom.json" >&2
    exit 1
  fi
  if [ "$(awk -v name="${filename}.sbom.json" \
    '$2 == name { count++ } END { print count + 0 }' "${manifest}")" -ne 1 ]; then
    echo "checksum manifest must contain ${filename}.sbom.json exactly once" >&2
    exit 1
  fi
done

(
  cd "${dist}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum --check "${manifest_name}"
  else
    shasum -a 256 -c "${manifest_name}"
  fi
)

for archive in \
  "${dist}/jobman-diagnose_${version}_darwin_amd64.tar.gz" \
  "${dist}/jobman-diagnose_${version}_darwin_arm64.tar.gz" \
  "${dist}/jobman-diagnose_${version}_linux_386.tar.gz" \
  "${dist}/jobman-diagnose_${version}_linux_amd64.tar.gz" \
  "${dist}/jobman-diagnose_${version}_linux_arm64.tar.gz"; do
  tar -tzf "${archive}" | grep -Fxq jobman-diagnose || {
    echo "$(basename "${archive}") does not contain jobman-diagnose" >&2
    exit 1
  }
done
for archive in \
  "${dist}/jobman-diagnose_${version}_windows_386.zip" \
  "${dist}/jobman-diagnose_${version}_windows_amd64.zip" \
  "${dist}/jobman-diagnose_${version}_windows_arm64.zip"; do
  unzip -Z1 "${archive}" | grep -Fxq jobman-diagnose.exe || {
    echo "$(basename "${archive}") does not contain jobman-diagnose.exe" >&2
    exit 1
  }
done

printf 'Verified Jobman Diagnose release %s: 8 archives, 9 native packages, 17 SBOMs.\n' \
  "${version}"
