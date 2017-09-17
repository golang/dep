package main

import (
	"flag"
	"github.com/golang/dep"
	"strings"
	"io"
	"os"
	"fmt"
	"compress/gzip"
	"archive/tar"
	"path/filepath"
	"net/http"
	"net/url"
	"path"
	"github.com/golang/dep/internal/gps"
	"github.com/pkg/errors"
	"crypto/sha256"
	"io/ioutil"
	"regexp"
	"github.com/Masterminds/vcs"
	"encoding/hex"
)

const publishShortHelp = `Publish project to registry`
const publishhLongHelp = `
Publishing is the process of uploading an immutable project version to the registry.
A Gopkg.reg with registry configuration is required.


Examples:

  dep publish 1.0.0         Publish using a specified version name
  dep publish -branch       Publish using the current vcs branch or tag (e.g. master, 1.0.1)
  dep publish -revision     Publish using the current vcs commit hash (e.g. abc123)

For more detailed usage examples, see dep publish -examples.
`
const publishExamples = `
dep publish -branch

	publish using the current vcs branch or tag (e.g. master, 1.0.1)


dep publish -revision

	publish using the current vcs commit hash (e.g. abc123)


dep publish <version_name>

	publish using a specified version name


dep publish -vendor github.com/go-yaml/yaml

	publish the vendor/github.com/go-yaml/yaml project


dep publish -vendor github.com/*

	publish any project under vendor/github.com


dep publish -vendor *

	publish all projects under vendor/
`

func (cmd *publishCommand) Name() string { return "publish" }
func (cmd *publishCommand) Args() string {
	return "[-vendor] [-branch | -revision] [<version>]"
}
func (cmd *publishCommand) ShortHelp() string { return publishShortHelp }
func (cmd *publishCommand) LongHelp() string  { return publishhLongHelp }
func (cmd *publishCommand) Hidden() bool      { return false }

func (cmd *publishCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.examples, "examples", false, "print detailed usage examples")
	fs.BoolVar(&cmd.branch, "branch", false, "print detailed usage examples")
	fs.BoolVar(&cmd.revision, "revision", false, "print detailed usage examples")
	fs.BoolVar(&cmd.vendor, "vendor", false, "print detailed usage examples")
}

type publishCommand struct {
	examples bool
	branch   bool
	revision bool
	vendor   bool
}

func (cmd *publishCommand) Run(ctx *dep.Ctx, args []string) error {
	if cmd.examples {
		ctx.Err.Println(strings.TrimSpace(publishExamples))
		return nil
	}

	p, err := ctx.LoadProject()
	if err != nil {
		return err
	}

	if p.RegistryConfig == nil {
		return errors.New("publishing requires a registry to be configured, use 'dep login' to set-up a registry")
	}

	if cmd.vendor {
		return cmd.publishVendor(ctx, p, args)
	}

	version := ""
	if len(args) > 0 {
		version = args[0]
	}
	return cmd.publish(ctx, string(p.ImportRoot), version, p.AbsRoot, p.RegistryConfig)
}

func (cmd *publishCommand) publish(ctx *dep.Ctx, name, version, path string, config gps.Registry) error {
	version, err := cmd.getVersion(version, path)
	if err != nil {
		return err
	}

	td, err := ioutil.TempDir(os.TempDir(), "dep")
	if err != nil {
		return errors.Wrap(err, "error while creating temp dir for writing manifest/lock/vendor")
	}
	defer os.RemoveAll(td)

	f, err := os.Create(filepath.Join(td, "project.tar.gz"))
	if err != nil {
		return err
	}
	defer f.Close()

	s := sha256.New()
	err = tarFiles(path, f, s)
	if err != nil {
		return err
	}

	content, err := os.Open(filepath.Join(td, "project.tar.gz"))
	if err != nil {
		return err
	}
	defer content.Close()

	ctx.Out.Printf("publishing project %s with version %s\n", name, version)
	return cmd.execUploadFile(name, version, hex.EncodeToString(s.Sum(nil)), content, config)
}

func (cmd *publishCommand) getVersion(version, projectPath string) (string, error) {
	if version != "" {
		return version, nil
	}
	if !cmd.revision && !cmd.branch {
		return "", errors.New("must provide version argument or -branch or -revision flags")
	}

	repo, err := vcs.NewRepo("", projectPath)
	if err != nil {
		return "", err
	}
	if cmd.revision {
		return repo.Version()
	}
	return repo.Current()
}

func isWildcard(s string) bool {
	return strings.ContainsAny(s, "*?")
}

func toRegexp(s string) string {
	r := strings.Replace(s, "*", ".*", -1)
	r = strings.Replace(r, "?", ".", -1)
	return r
}

func (cmd *publishCommand) publishVendor(ctx *dep.Ctx, p *dep.Project, args []string) error {
	if len(args) == 0 {
		return errors.New("must provide projects argument")
	}

	if cmd.branch || cmd.revision {
		return errors.New("cannot provide -branch or -revision with -vendor flag")
	}

	if isWildcard(args[0]) && len(args) > 1 {
		return errors.New("cannot provide version argument with wildcard project argument")
	}

	pRegexp, err := regexp.Compile(toRegexp(args[0]))
	if err != nil {
		return err
	}

	if p.Lock == nil {
		return errors.New("cannot publish vendor projects - current project has no Goconf.lock file")
	}

	foundProject := false
	for _, v := range p.Lock.Projects() {
		pName := string(v.Ident().ProjectRoot)
		if pRegexp.MatchString(pName) {
			version := v.Version().String()
			if len(args) > 1 {
				version = args[1]
			}
			pPath := filepath.Join(p.AbsRoot, "vendor", pName)
			err = cmd.publish(ctx, pName, version, pPath, p.RegistryConfig)
			if err != nil {
				return err
			}
			foundProject = true
		}
	}
	if !foundProject {
		return errors.New("cannot find matching vendor projects for: " + args[0])
	}

	return nil
}

// Tar takes a source and variable writers and walks 'source' writing each file
// found to the tar writer; the purpose for accepting multiple writers is to allow
// for multiple outputs (for example a file, or md5 hash)
func tarFiles(src string, writers ...io.Writer) error {

	// ensure the src actually exists before trying to tar it
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("Unable to tar files - %v", err.Error())
	}

	mw := io.MultiWriter(writers...)

	gzw := gzip.NewWriter(mw)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	// walk path
	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {

		// return on any error
		if err != nil {
			return err
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, src, "", -1), string(filepath.Separator))

		// write the header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// return on directories since there will be no content to tar
		if fi.Mode().IsDir() {
			return nil
		}

		// open files for taring
		f, err := os.Open(file)
		defer f.Close()
		if err != nil {
			return err
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		return nil
	})
}
func (cmd *publishCommand) execUploadFile(name, version string, sha256 string, content io.Reader, r gps.Registry) error {
	u, err := url.Parse(r.URL())
	if err != nil {
		return err
	}

	u.Path = path.Join(u.Path, "api/v1/projects", url.PathEscape(name), version)
	req, err := http.NewRequest("PUT", u.String(), content)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "BEARER "+r.Token())
	req.Header.Set("X-Checksum-Sha256", sha256)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return errors.Errorf("%s %s", u.String(), http.StatusText(resp.StatusCode))
	}
	return nil
}
