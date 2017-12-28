package gps

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/golang/dep/gps/pkgtree"
	"github.com/golang/dep/internal/fs"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
)

// Registry configuration interface
// Set env vars: DEPREGISTRYURL, DEPREGISTRYTOKEN
type Registry interface {
	URL() string
	Token() string
}

// NewRegistryConfig creates new registry config using provided url and token
func NewRegistryConfig(url *url.URL, token string) Registry {
	return &RegitryConfig{url: url.String(), token: token}
}

// RegistryConfig holds url and token of Golang registry
type RegitryConfig struct {
	url   string
	token string
}

// URL returns registry URL
func (s *RegitryConfig) URL() string {
	return s.url
}

// Token returns registry token
func (s *RegitryConfig) Token() string {
	return s.token
}

type registrySource struct {
	path            string
	url             string
	token           string
	sourceCachePath string
}

func (s *registrySource) URL() string {
	return s.url
}

func (s *registrySource) Token() string {
	return s.token
}

func (s *registrySource) existsLocally(ctx context.Context) bool {
	return false
}

func (s *registrySource) existsUpstream(ctx context.Context) bool {
	return true
}

func (s *registrySource) upstreamURL() string {
	return path.Join(s.url + s.path)
}

func (s *registrySource) initLocal(ctx context.Context) error {
	return nil
}

func (s *registrySource) updateLocal(ctx context.Context) error {
	return nil
}

// Get version from registry
func (s *registrySource) execGetVersions() (*rawVersions, error) {
	u, err := url.Parse(s.url)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "api/v1/projects", url.PathEscape(s.path), "info")
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("%s %s", u.String(), http.StatusText(resp.StatusCode))
	}

	var bytes []byte
	bytes, err = ioutil.ReadAll(resp.Body)
	var versionsResp rawVersions
	err = json.Unmarshal(bytes, &versionsResp)
	return &versionsResp, err
}

func (s *registrySource) execDownloadDependency(ctx context.Context, pr ProjectRoot, r Revision) (*http.Response, error) {
	u, err := url.Parse(s.url)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "api/v1/projects", url.PathEscape(s.path), "versions", r.String())
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("%s %s", u, http.StatusText(resp.StatusCode))
	}
	return resp, nil
}

type rawVersions struct {
	Versions map[string]rawPublished `json:"versions"`
}
type rawPublished struct {
	Published string `json:"published"`
}

func (s *registrySource) listVersions(ctx context.Context) (vlist []PairedVersion, err error) {
	vers := []PairedVersion{}
	rawResp, err := s.execGetVersions()
	if err != nil {
		return vers, err
	}

	for k := range rawResp.Versions {
		vers = append(vers, NewVersion(k).Pair(Revision(k)))
	}
	return vers, nil
}

func (s *registrySource) getManifestAndLock(ctx context.Context, pr ProjectRoot, r Revision, an ProjectAnalyzer) (Manifest, Lock, error) {
	m, l, err := an.DeriveManifestAndLock(s.sourceCachePath, pr)
	if err != nil {
		return nil, nil, err
	}

	if l != nil && l != Lock(nil) {
		l = prepLock(l)
	}

	return prepManifest(m), l, nil
}

func (s *registrySource) listPackages(ctx context.Context, pr ProjectRoot, r Revision) (ptree pkgtree.PackageTree, err error) {
	resp, err := s.execDownloadDependency(ctx, pr, r)
	if err != nil {
		return pkgtree.PackageTree{}, err
	}
	defer resp.Body.Close()

	h := sha256.New()
	tee := io.TeeReader(resp.Body, h)

	err = extractDependency(tee, s.sourceCachePath)
	if err != nil {
		return pkgtree.PackageTree{}, err
	}

	if hex.EncodeToString(h.Sum(nil)) != resp.Header.Get("X-Checksum-Sha256") {
		return pkgtree.PackageTree{}, errors.Errorf("sha256 checksum validation failed for %s %s", s.path, r)
	}

	return pkgtree.ListPackages(s.sourceCachePath, string(pr))
}

func extractDependency(r io.Reader, target string) error {
	gzr, err := gzip.NewReader(r)
	defer gzr.Close()
	if err != nil {
		return err
	}

	// Remove other versions of the same dependency.
	if err = os.RemoveAll(target); err != nil {
		return err
	}
	if err = os.MkdirAll(target, 0755); err != nil {
		return err
	}

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		filePath := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(filePath, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *registrySource) revisionPresentIn(r Revision) (bool, error) {
	return false, nil
}

func (s *registrySource) exportRevisionTo(ctx context.Context, r Revision, to string) error {
	if err := os.MkdirAll(s.sourceCachePath, 0755); err != nil {
		return err
	}
	return fs.CopyDir(s.sourceCachePath, to)
}

func (s *registrySource) sourceType() string {
	return "registry"
}

type maybeRegistrySource struct {
	path  string
	url   *url.URL
	token string
}

func (m maybeRegistrySource) possibleURLs() []*url.URL {
	return []*url.URL{m.url}
}

func (s *registrySource) disambiguateRevision(ctx context.Context, r Revision) (Revision, error) {
	return r, nil
}

func (m maybeRegistrySource) try(ctx context.Context, cachedir string, c singleSourceCache, superv *supervisor) (source, sourceState, error) {
	registry, err := NewRegistrySource(m.url.String(), m.token, m.path, cachedir)
	if err != nil {
		return nil, 0, err
	}
	return registry, sourceIsSetUp | sourceExistsUpstream, nil
}

// NewRegistrySource creates new registry source
func NewRegistrySource(rURL, token, rPath, cachedir string) (source, error) {
	return &registrySource{
		path:            rPath,
		url:             rURL,
		token:           token,
		sourceCachePath: sourceCachePath(cachedir, rURL),
	}, nil
}
