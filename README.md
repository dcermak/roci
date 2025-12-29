# ROCI - **R**PM from **OCI**

`roci` is a packaging utility that converts OCI images to RPM packages. It is
intended to reduce the barrier to entry when building system packages.

## Example

```Dockerfile
ARG VERSION=4.3
ARG DIST
FROM fedora-rpm-buildroot:${DIST} as buildrequires

WORKDIR /src/
COPY poke-${VERSION}.tar.gz .
COPY poke-${VERSION}.tar.gz.sig .

RUN dnf -y install emacs gcc gc-devel nbdkit make

FROM localhost/poke-buildrequires as builder

RUN rpmdev-extract poke-${VERSION}.tar.gz
WORKDIR /src/poke-${VERSION}
RUN ./configure; \
    make build; \
    make check

RUN make install PREFIX=/usr/local

FROM fedora-rpm-buildroot:${DIST} as poke

LABEL org.opencontainers.image.version=${VERSION}
LABEL org.opencontainers.image.url="https://www.jemarch.net/poke"
LABEL org.opencontainers.image.title="Extensible editor for structured binary data"
LABEL org.opencontainers.image.description="GNU poke is an interactive, extensible editor for binary data."
LABEL org.opencontainers.image.licenses="GPL-3.0-or-later AND GFDL-1.3-no-invariants-or-later"

COPY --from=builder /usr/local/bin /usr/bin
```

## configuration file

roci can read all rpm package metadata from a configuration file. This is
necessary for certain rpm settings that cannot be set as labels, e.g. `Requires`
or scriptlets.

```yaml
Name: "poke"
Version: 4.3
Release: $AUTORELEASE
Summary: "Extensible editor for structured binary data"
Description: |
   GNU poke is an interactive, extensible editor for binary data. Not
   limited to editing basic entities such as bits and bytes, it provides
   a full-fledged procedural, interactive programming language designed
   to describe data structures and to operate on them.

URL: "https://www.jemarch.net/poke"
License: "GPL-3.0-or-later AND GFDL-1.3-no-invariants-or-later"

Requires:
  - "poke-data = $VERSION-$RELEASE"
  - "poke-libs = $VERSION-$RELEASE"

Requires(preun): ""

Preun: "/usr/sbin/alternatives --remove  poke /usr/bin/poke || :"

package:
  poke-devel:
    Name: "poke-devel"
    Summary: "Development files for poke"
    Requires:
      - "poke = $VERSION-$RELEASE"
    Description: |
       The poke-devel package contains libraries and header files for
       developing applications that use poke.
```

Each output stage must be defined in the config file, except for the main
package, which is always built.

The `yaml` configuration file **must** be called `$pkg-name.yaml` where
`$pkg-name` is the main package's name (this should be the same as the directory)


## Conventions used

The following OCI annotations/labels are directly converted into RPM tags:

- `org.opencontainers.image.version` -> `Version`
- `org.opencontainers.image.url` -> `URL`
- `org.opencontainers.image.title` -> `Sumary`
- `org.opencontainers.image.description` -> `Description`
- `org.opencontainers.image.licenses` -> `License`

Additionally, we invent our own to address the remaining missing RPM tags:

- `org.rpm.name` -> `Name`
- `org.rpm.epoch` -> `Epoch`
- `org.rpm.release` -> `Release`


## Stage names

To support consistent building of packages from Containerfiles, the following
stages **must** be present in the `Containerfile`:

1. `buildrequires` - the first stage where all sources are copied into the
   container build/buildroot and dependencies are installed. This stage runs
   with network access to install build dependencies. Package sources must be
   pre-downloaded and `COPY`'d into the buildroot.
   The result of this stage is tagged as `$name-buildrequires`.
   This stage **must** `${distro}-rpm-buildroot:$DIST` as `FROM`.

2. `build` - the stage that actually builds the package and runs the tests. It
   must use `localhost/$name-buildrequires` as the `FROM` image.

3. `$name` and `$subpkg-name` stages - the build results are copied into these
   stages. These stages **must** use `${distro}-rpm-buildroot:$DIST` as `FROM`
   and 


## Assembling RPM subpackages

Subpackages are defined as build stages in the `Containerfile`, but they must be
named as dictionary entries in the `package` key. Stages not defined in the
config file, are ignored.


## AutoReqProv



## RPM Spec patterns in `Containerfile`

Dockerfiles do not support typical rpm constructs like `%bcond` or
macros. However, certain 


# Usage

```ShellSession
$ roci build [path/to/dist-git-dir]


```
