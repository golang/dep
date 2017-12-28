package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const TokenAuth = "2erygdasE45rty5JKwewrr75cb15rdeE"

var tokenResp = "{\"token\": \"" + TokenAuth + "\"}"

type rawVersions struct {
	Versions map[string]rawPublished `json:"versions"`
}
type rawPublished struct {
	Published string `json:"published"`
}

type rawProjectName struct {
	ProjectName string `json:"project_name"`
}

func verifyToken(r *http.Request) error {
	tokens, ok := r.Header["Authorization"]
	if ok && len(tokens) >= 1 {
		token := tokens[0]
		token = strings.TrimPrefix(token, "Bearer ")
		if token == TokenAuth {
			return nil
		}
	}

	return errors.New(http.StatusText(http.StatusUnauthorized))
}

func getSourcesPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "..", "..", "internal", "test", "registry", "sources"), nil
}

func getVersions(dependency string) rawVersions {
	versions := rawVersions{Versions: map[string]rawPublished{}}
	sourcesPath, err := getSourcesPath()
	if err != nil {
		return versions
	}

	files, err := ioutil.ReadDir(filepath.Join(sourcesPath, dependency))
	if err != nil {
		return versions
	}
	for _, v := range files {
		if v.IsDir() {
			versions.Versions[v.Name()] = rawPublished{Published: v.ModTime().String()}
		}
	}
	return versions
}

func token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	fmt.Fprint(w, tokenResp) // send data to client side
}

func getInfo(w http.ResponseWriter, r *http.Request, projectName string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	if err := verifyToken(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	versions := getVersions(projectName)
	if len(versions.Versions) == 0 {
		http.NotFound(w, r)
		return
	}
	requestContent, err := json.Marshal(versions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(requestContent)
}

func getDependency(w http.ResponseWriter, r *http.Request, projectName, version string) {
	sourcesPath, err := getSourcesPath()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	f, err := os.OpenFile(filepath.Join(sourcesPath, projectName, version, projectName+".tar.gz"), os.O_RDONLY, 0666)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h := sha256.New()
	h.Write(b)

	w.Header().Add("X-Go-Project-Name", strings.Replace(projectName, "%2F", "/", -1))
	w.Header().Add("X-Go-Project-Version", version)
	w.Header().Add("X-Checksum-Sha256", hex.EncodeToString(h.Sum(nil)))
	w.Write(b)
}

func getProject(w http.ResponseWriter, r *http.Request) {
	if err := verifyToken(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	dep := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
	pathComponents := strings.Split(dep, "/")

	switch pathComponents[1] {
	case "info":
		getInfo(w, r, pathComponents[0])
		return
	case "versions":
		getDependency(w, r, pathComponents[0], pathComponents[2])
		return
	}
	http.NotFound(w, r)
}

func putProject(w http.ResponseWriter, r *http.Request) {
	if err := verifyToken(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	// validate content with sha256 header
	h := sha256.New()
	h.Write(content)

	if hex.EncodeToString(h.Sum(nil)) != r.Header.Get("X-Checksum-Sha256") {
		http.Error(w, "sha256 checksum validation failed", http.StatusForbidden)
	}
	// Do nothing with the received dependency
	return
}

func dependency(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getProject(w, r)
	case http.MethodPut:
		putProject(w, r)
	case http.MethodHead:
		projectName(w, r)
	default:
		http.NotFound(w, r)
	}
}

// Simple impl to get project name from import path
func projectName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	importPath := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
	sourcePath, err := getSourcesPath()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	files, err := ioutil.ReadDir(sourcePath)
	if err != nil {
		log.Fatal(err)
	}

	for _, v := range files {
		if isRootProject(v.Name(), importPath) {
			w.Header().Set("X-Go-Project-Name", strings.Replace(v.Name(), "%2F", "/", -1))
			return
		}
	}

	http.Error(w, "could not find project name", http.StatusNotFound)
	return
}

func isRootProject(projectName, importPath string) bool {
	suffix := strings.TrimPrefix(projectName, importPath)
	return suffix == "" || strings.HasPrefix(suffix, "%2F")
}

func notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// SetupAndRun starts the registry mock for testing purposes
func SetupAndRun(addr string) error {
	http.HandleFunc("/api/v1/auth/token", token)
	http.HandleFunc("/api/v1/projects/", dependency)
	http.HandleFunc("/", notFound)
	return http.ListenAndServe(addr, nil)
}
