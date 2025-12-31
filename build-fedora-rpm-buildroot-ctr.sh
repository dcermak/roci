#!/bin/bash

set -euo pipefail

tag="${1:-rawhide}"

ctr=$(buildah from registry.fedoraproject.org/fedora:$tag)

trap "buildah rm \"$ctr\" > /dev/null" EXIT

# we need to install redhat-rpm-config first before we try to evaluate the
# macros which contain the compiler flags
buildah run "$ctr" dnf -y install rpmdevtools rpm-build redhat-rpm-config 'rpm_macro(build_rustflags)'
buildah run "$ctr" dnf -y clean all

RPM_CFLAGS=$(buildah run "$ctr" rpm --eval "%{build_cflags}")
RPM_CXXFLAGS=$(buildah run "$ctr" rpm --eval "%{build_cxxflags}")
RPM_FFLAGS=$(buildah run "$ctr" rpm --eval "%{build_fflags}")
RPM_VALAFLAGS=$(buildah run "$ctr" rpm --eval "%{build_valaflags}")
RPM_RUSTFLAGS=$(buildah run "$ctr" rpm --eval "%{build_rustflags}")
RPM_LDFLAGS=$(buildah run "$ctr" rpm --eval "%{build_ldflags}")
RPM_CC=$(buildah run "$ctr" rpm --eval "%{build_cc}")
RPM_CXX=$(buildah run "$ctr" rpm --eval "%{build_cxx}")

buildah config --env CFLAGS="$RPM_CFLAGS" "$ctr"
buildah config --env CXXFLAGS="$RPM_CXXFLAGS" "$ctr"
buildah config --env FFLAGS="$RPM_FFLAGS" "$ctr"
buildah config --env FCFLAGS="$RPM_FFLAGS" "$ctr"
buildah config --env VALAFLAGS="$RPM_VALAFLAGS" "$ctr"
buildah config --env RUSTFLAGS="$RPM_RUSTFLAGS" "$ctr"
buildah config --env LDFLAGS="$RPM_LDFLAGS" "$ctr"
buildah config --env CC="$RPM_CC" "$ctr"
buildah config --env CXX="$RPM_CXX" "$ctr"

buildah commit "$ctr" "fedora-rpm-buildroot:$tag"
