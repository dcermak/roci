package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containers/buildah"
	"github.com/containers/buildah/define"
	"github.com/containers/buildah/imagebuildah"
	"github.com/google/rpmpack"
	"github.com/urfave/cli/v3"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/pkg/blobinfocache/none"
	"go.podman.io/image/v5/pkg/compression"
	imgStorage "go.podman.io/image/v5/storage"
	"go.podman.io/image/v5/types"
	"go.podman.io/storage"
	"go.podman.io/storage/pkg/reexec"
	"go.podman.io/storage/pkg/unshare"

	roci "github.com/dcermak/roci/pkg"
)

func main() {
	if reexec.Init() {
		return
	}

	cmd := &cli.Command{
		Name:  "roci",
		Usage: "Build RPM packages from OCI images",
		Commands: []*cli.Command{
			{
				Name:      "build",
				Usage:     "Build RPM package from OCI image",
				ArgsUsage: "dist-git-dir",
				Action:    buildCommand,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "file",
						Aliases: []string{"f"},
						Value:   "Containerfile",
						Usage:   "filename of to the Containerfile/Dockerfile, defaults to Containerfile",
					},
					&cli.StringFlag{
						Name:    "yaml-file",
						Aliases: []string{"c"},
						Usage:   "name of the yaml config file",
					},
					&cli.StringFlag{
						Name:    "release",
						Aliases: []string{"r"},
						Value:   "",
						Usage:   "Distribution release to target",
					},
				},
			},
		},
	}

	unshare.MaybeReexecUsingUserNamespace(true)

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

type Build struct {
	store       storage.Store
	config      roci.Config
	distGit     string
	buildRecipe string
	distTag     string
	ctx         context.Context
}

func releaseToDistTag(release string) string {
	switch {
	case strings.HasPrefix(release, "f"):
		return release[1:]
	case strings.HasPrefix(release, "el"):
		return release[2:]
	default:
		// TODO openSUSE, SLES, mandriva
		return ""
	}
}

// NewBuild creates a new Build from the
func NewBuild(ctx context.Context, cmd *cli.Command) (*Build, error) {
	// we must store the dist-git dir as the absolute path, as we will be
	// using it as build context in the user namespace. There we loose the
	// current working directory and a relative path will resolve wrongly
	distGitDir, err := filepath.Abs(cmd.Args().First())
	if err != nil {
		return nil, err
	}

	yamlFileName := cmd.String("yaml-file")
	// no yaml file provided? => glob it and pick first match
	if yamlFileName == "" {
		matches, err := filepath.Glob(path.Join(distGitDir, "*.yaml"))
		if err != nil {
			return nil, err
		}
		if matches == nil || len(matches) == 0 {
			return nil, errors.New("No yaml file in dist-git dir")
		}

		yamlFileName = matches[0]
	} else {
		yamlFileName = filepath.Join(distGitDir, yamlFileName)
	}

	// actual config load
	config, err := roci.LoadConfig(yamlFileName)
	if err != nil {
		return nil, err
	}

	// container image store
	storeOptions, err := storage.DefaultStoreOptions()
	if err != nil {
		return nil, err
	}
	store, err := storage.GetStore(storeOptions)
	if err != nil {
		return nil, err
	}

	return &Build{
		store:       store,
		config:      *config,
		distGit:     distGitDir,
		buildRecipe: cmd.String("file"),
		distTag:     releaseToDistTag(cmd.String("release")),
		ctx:         ctx,
	}, nil
}

// commonBuildArgs returns a map of the build arguments added to the build:
// DIST: disttag
// NAME: package name
// RELEASE: release
// VERSION: package version
// only build arguments with values != "" are added
func (b *Build) commonBuildArgs() map[string]string {
	data := []struct {
		Name  string
		Value string
	}{
		{"DIST", b.distTag},
		{"VERSION", b.config.Version},
		{"NAME", b.config.Name},
		{"RELEASE", b.config.Release},
	}
	args := make(map[string]string)
	for _, d := range data {
		if d.Value != "" {
			args[d.Name] = d.Value
		}
	}
	return args
}

func (b *Build) buildStage(targetStage string, outputTag string, withNetwork bool) (string, reference.Canonical, error) {
	// FIXME: need to set sourcedateepoch here
	buildOptions := define.BuildOptions{
		Target: targetStage,
		Output: outputTag,

		Args:             b.commonBuildArgs(),
		ContextDirectory: b.distGit,

		// these two must be false so that the layers on top of the base
		// image are squashed
		NoCache: false,
		Layers:  false,

		// emit useful output
		Out: os.Stdout,
		Err: os.Stderr,
		// Ensure CommonBuildOpts is initialized (though BuildDockerfiles handles nil)
		CommonBuildOpts: &define.CommonBuildOptions{},
	}
	if !withNetwork {
		buildOptions.ConfigureNetwork = define.NetworkDisabled
	}

	return imagebuildah.BuildDockerfiles(
		b.ctx, b.store, buildOptions, b.buildRecipe,
	)
}

func (b *Build) executeBuildRequires() (string, reference.Canonical, error) {
	return b.buildStage("buildrequires", fmt.Sprintf("%s-buildrequires", b.config.Name), true)
}

func (b *Build) executeBuild() (string, reference.Canonical, error) {
	return b.buildStage("build", fmt.Sprintf("%s-build", b.config.Name), false)
}

// ImageFromId returns an image from the image store given the supplied id.
// The caller *must* call close on the returned imageCloser if error is non-nil
func (b *Build) ImageFromId(id string) (types.ImageCloser, error) {
	storageRef, err := imgStorage.Transport.ParseStoreReference(b.store, id)
	if err != nil {
		return nil, err
	}

	ref, err := storageRef.NewImage(b.ctx, nil)
	if err != nil {
		return nil, err
	}
	return ref, nil
}

// AddRpmDependenciesFromConfig extracts the string representation of dependency
// information (like requires, provides) from the package `rpmPkg` and appends
// them to the existing metadata of `m`. The modified rpm metadata are returned
// on success.
// If any of the dependencies cannot be converted into a proper relation, then
// an error is returned.
func AddRpmDependenciesFromConfig(m rpmpack.RPMMetaData, rpmPkg roci.RpmPackage) (rpmpack.RPMMetaData, error) {
	strToRelations := func(deps []string) ([]*rpmpack.Relation, error) {
		rels := make([]*rpmpack.Relation, len(deps))
		for i, d := range deps {
			r, err := rpmpack.NewRelation(d)
			if err != nil {
				return nil, err
			}
			rels[i] = r
		}
		return rels, nil
	}

	provides, err := strToRelations(rpmPkg.Provides)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}

	var allRequires []string
	allRequires = append(allRequires, rpmPkg.Requires...)
	allRequires = append(allRequires, rpmPkg.RequiresPre...)
	allRequires = append(allRequires, rpmPkg.RequiresPost...)
	allRequires = append(allRequires, rpmPkg.RequiresPreUn...)
	allRequires = append(allRequires, rpmPkg.RequiresPostUn...)
	allRequires = append(allRequires, rpmPkg.RequiresPreTrans...)
	allRequires = append(allRequires, rpmPkg.RequiresPostTrans...)
	allRequires = append(allRequires, rpmPkg.RequiresVerify...)
	allRequires = append(allRequires, rpmPkg.RequiresInterp...)
	allRequires = append(allRequires, rpmPkg.RequiresMeta...)

	requires, err := strToRelations(allRequires)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}

	obsoletes, err := strToRelations(rpmPkg.Obsoletes)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}
	conflicts, err := strToRelations(rpmPkg.Conflicts)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}
	recommends, err := strToRelations(rpmPkg.Recommends)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}
	suggests, err := strToRelations(rpmPkg.Suggests)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}

	m.Provides = append(m.Provides, provides...)
	m.Requires = append(m.Requires, requires...)
	m.Obsoletes = append(m.Obsoletes, obsoletes...)
	m.Conflicts = append(m.Conflicts, conflicts...)
	m.Recommends = append(m.Recommends, recommends...)
	m.Suggests = append(m.Suggests, suggests...)

	return m, nil
}

// AddRpmMetadataFromImageLabels extracts the labels from the supplied image and
// sets the version, URL, title, description, license, name, release and epoch
// fields from image labels.
// Set the buildtime to the image creation time
func (b *Build) AddRpmMetadataFromImageLabels(m rpmpack.RPMMetaData, img types.Image) (rpmpack.RPMMetaData, error) {
	inspect, err := img.Inspect(b.ctx)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}

	if ver, ok := inspect.Labels["org.opencontainers.image.version"]; ok {
		m.Version = ver
	}
	if url, ok := inspect.Labels["org.opencontainers.image.url"]; ok {
		m.URL = url
	}
	if title, ok := inspect.Labels["org.opencontainers.image.title"]; ok {
		m.Summary = title
	}
	if description, ok := inspect.Labels["org.opencontainers.image.description"]; ok {
		m.Description = description
	}
	if licenses, ok := inspect.Labels["org.opencontainers.image.licenses"]; ok {
		m.Licence = licenses
	}

	// FIXME: probably pointless?
	if name, ok := inspect.Labels["org.rpm.name"]; ok {
		m.Name = name
	}
	if release, ok := inspect.Labels["org.rpm.release"]; ok {
		m.Release = release
	}
	if epoch, ok := inspect.Labels["org.rpm.epoch"]; ok {
		i, err := strconv.ParseInt(epoch, 10, 32)
		if err != nil {
			return rpmpack.RPMMetaData{}, err
		}
		m.Epoch = uint32(i)
	}
	if inspect.Created != nil {
		m.BuildTime = *inspect.Created
	}
	return m, nil
}

// WalkTopLayerTree decompresses the top layer of the supplied image `img` and
// invokes `callback` on each node in the layer.
//
// `callback` can abort walking the tree by returning an error, then this
// function immediately returns said error.
func (b *Build) WalkTopLayerTree(img types.Image, callback func(path string, hdr *tar.Header, contents []byte) error) error {
	blobInfo, err := img.LayerInfosForCopy(b.ctx)
	if err != nil {
		return err
	}
	src, err := img.Reference().NewImageSource(b.ctx, nil)
	if err != nil {
		return err
	}
	defer src.Close()

	// blobInfo is ordered from the root layer to the top most layer
	// in theory there should be just two layer though
	topLayerBlob := blobInfo[len(blobInfo)-1]
	blob, _, err := src.GetBlob(b.ctx, topLayerBlob, none.NoCache)
	if err != nil {
		return err
	}
	defer blob.Close()

	decompressedStream, _, err := compression.AutoDecompress(blob)
	if err != nil {
		return err
	}
	defer decompressedStream.Close()

	tarRdr := tar.NewReader(decompressedStream)
	for {
		hdr, err := tarRdr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path, err := filepath.Abs(filepath.Join("/", hdr.Name))
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeLink | tar.TypeSymlink | tar.TypeChar | tar.TypeBlock | tar.TypeDir | tar.TypeFifo:
			err = callback(path, hdr, nil)
		case tar.TypeReg:
			b := make([]byte, hdr.Size)
			_, err = tarRdr.Read(b)
			if err != io.EOF {
				return err
			}
			err = callback(path, hdr, b)
		}

		if err != nil {
			return err
		}

	}

	return nil
}

// AutoReqProv calculates the automated Requires, Provides, etc of the files in
// `filelist` and returns them in the RPMMetaData struct
func (b *Build) AutoReqProv(imgId string, filelist []string) (rpmpack.RPMMetaData, error) {
	builderOpts := buildah.BuilderOptions{
		FromImage: imgId,
	}
	builder, err := buildah.NewBuilder(b.ctx, b.store, builderOpts)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}
	defer builder.Delete() // Clean up the working container when done

	buff := bytes.Buffer{}
	runOptions := buildah.RunOptions{
		Stdout: &buff,
		Stderr: os.Stderr,
		// we must set RPM_BUILD_ROOT as otherwise ELF libraries cannot
		// be processed by rpmdeps
		// FIXME: this is actually not entirely correct, we should define this as a build ARG somewhere
		Env:      []string{"RPM_BUILD_ROOT=/"},
		Terminal: buildah.WithoutTerminal,
	}

	cmd := append([]string{"/usr/lib/rpm/rpmdeps", "--alldeps"}, filelist...)
	err = builder.Run(cmd, runOptions)
	if err != nil {
		return rpmpack.RPMMetaData{}, err
	}

	// don't commit the result! we just want the contents of buff to
	// calculate the dependencies
	return roci.ParseRpmdepsOutput(buff.String())
}

func (b *Build) RpmFromLayer(id string, rpmPkg roci.RpmPackage) (*rpmpack.RPM, error) {
	img, err := b.ImageFromId(id)
	if err != nil {
		return nil, err
	}
	defer img.Close()

	metaData := rpmpack.RPMMetaData{
		// what else?
		OS: "linux",
		// TODO: buildhost?

		// TODO: should be configurable
		Compressor: "zstd",

		// unlikely to be set, but will certainly not be overridden
		// below
		Group:    rpmPkg.Group,
		Packager: rpmPkg.Packager,
		Vendor:   rpmPkg.Vendor,
	}

	metaData, err = b.AddRpmMetadataFromImageLabels(metaData, img)
	if err != nil {
		return nil, err
	}

	// now get the remaining metadata from the rpmPkg struct

	// yes, the following code makes my eyes bleed, but the only way to
	// do this in a controlled loop is with reflection, and that makes
	// my eyes bleed even more
	// So let's do it field, by field. _sight_
	if rpmPkg.Name != "" {
		metaData.Name = rpmPkg.Name
	}
	if rpmPkg.Version != "" {
		metaData.Version = rpmPkg.Version
	}
	if rpmPkg.Release != "" {
		metaData.Release = rpmPkg.Release
	}
	if rpmPkg.Summary != "" {
		metaData.Summary = rpmPkg.Summary
	}
	if rpmPkg.Description != "" {
		metaData.Description = rpmPkg.Description
	}
	if rpmPkg.URL != "" {
		metaData.URL = rpmPkg.URL
	}

	// different spelling 😡
	if rpmPkg.License != "" {
		metaData.Licence = rpmPkg.License
	}
	// and different types 💣️
	if rpmPkg.Epoch != 0 {
		metaData.Epoch = uint32(rpmPkg.Epoch)
	}

	m, err := AddRpmDependenciesFromConfig(metaData, rpmPkg)
	if err != nil {
		return nil, err
	}

	// Assembly time!!
	rpm, err := rpmpack.NewRPM(m)
	if err != nil {
		return nil, err
	}

	filelist := make([]string, 0)
	err = b.WalkTopLayerTree(img, func(path string, hdr *tar.Header, body []byte) error {
		filelist = append(filelist, path)

		f := rpmpack.RPMFile{
			Name:  path,
			Mode:  uint(hdr.Mode),
			Owner: hdr.Uname,
			Group: hdr.Gname,
			MTime: uint32(hdr.ModTime.Unix()),
			Body:  body,
			// FIXME: this is generally wrong
			Type: rpmpack.GenericFile,
		}
		rpm.AddFile(f)
		return nil
	})
	if err != nil {
		return nil, err
	}

	autoMetadata, err := b.AutoReqProv(id, filelist)
	if err != nil {
		return nil, err
	}

	rpm.Provides = append(rpm.Provides, autoMetadata.Provides...)
	rpm.Requires = append(rpm.Requires, autoMetadata.Requires...)
	rpm.Recommends = append(rpm.Recommends, autoMetadata.Recommends...)
	rpm.Obsoletes = append(rpm.Obsoletes, autoMetadata.Obsoletes...)
	rpm.Suggests = append(rpm.Suggests, autoMetadata.Suggests...)
	rpm.Conflicts = append(rpm.Conflicts, autoMetadata.Conflicts...)

	return rpm, nil
}

func buildCommand(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 {
		return fmt.Errorf("dist-git directory path is required")
	}

	build, err := NewBuild(ctx, cmd)
	if err != nil {
		return err
	}

	if _, _, err := build.executeBuildRequires(); err != nil {
		return err
	}

	if _, _, err := build.executeBuild(); err != nil {
		return err
	}

	// main package
	if id, _, err := build.buildStage(build.config.Name, build.config.Name, false); err != nil {
		return err
	} else {
		rpm, err := build.RpmFromLayer(id, build.config.RpmPackage)
		if err != nil {
			return err
		}
		rpmPath := filepath.Join(build.distGit, build.config.Name+".rpm")
		f, err := os.Create(rpmPath)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := rpm.Write(f); err != nil {
			return err
		}

	}

	for _, v := range build.config.Package {
		if _, _, err := build.buildStage(v.Name, v.Name, false); err != nil {
			return err
		}
	}

	return nil
}
