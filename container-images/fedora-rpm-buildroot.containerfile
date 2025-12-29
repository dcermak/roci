ARG DIST
FROM registry.fedoraproject.org/fedora:${DIST}

RUN dnf -y install rpmdevtools rpm-build; dnf -y clean all;
