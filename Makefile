containers:
	buildah bud --layers --build-arg=DIST=43 \
		-f container-images/fedora-rpm-buildroot.containerfile \
		--tag fedora-rpm-buildroot:43
	buildah bud --layers --build-arg=DIST=42 \
		-f container-images/fedora-rpm-buildroot.containerfile \
		--tag fedora-rpm-buildroot:42
