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

const TOKEN_AUTH = "2erygdasE45rty5JKwewrr75cb15rdeE"

var tokenResp = "{\"token\": \"" + TOKEN_AUTH + "\"}"

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
		token = strings.TrimPrefix(token, "BEARER ")
		if token == TOKEN_AUTH {
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

func versions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	if err := verifyToken(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	depName := strings.TrimPrefix(r.URL.Path, "/api/v1/versions/")
	versions := getVersions(depName)
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

func getDependency(w http.ResponseWriter, r *http.Request) {
	if err := verifyToken(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	dep := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
	nameVer := strings.Split(dep, "/")
	if len(nameVer) != 2 {
		http.NotFound(w, r)
		return
	}

	sourcesPath, err := getSourcesPath()
	if err != nil {
		http.NotFound(w, r)
		return
	}

	f, err := os.OpenFile(filepath.Join(sourcesPath, dep, nameVer[0]+".tar.gz"), os.O_RDONLY, 0666)
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

	w.Header().Add("X-Go-Project-Name", strings.Replace(nameVer[0], "%2F", "/", -1))
	w.Header().Add("X-Go-Project-Version", nameVer[1])
	w.Header().Add("X-Checksum-Sha256", hex.EncodeToString(h.Sum(nil)))
	w.Write(b)
}

func putDependency(w http.ResponseWriter, r *http.Request) {
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
		getDependency(w, r)
	case http.MethodPut:
		putDependency(w, r)
	default:
		http.NotFound(w, r)
	}
}

// Simple impl to get project name from import path
func projectName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	importPath := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/root/")
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
			projectName := &rawProjectName{ProjectName: strings.Replace(v.Name(), "%2F", "/", -1)}
			requestContent, err := json.Marshal(projectName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Write(requestContent)
			return
		}
	}

	http.Error(w, "could not find project name", http.StatusInternalServerError)
	return
}

func isRootProject(projectName, importPath string) bool {
	suffix := strings.TrimPrefix(projectName, importPath)
	return suffix == "" || strings.HasPrefix(suffix, "%2F")
}

func notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func SetupAndRun(addr string) error {
	http.HandleFunc("/api/v1/auth/token", token)
	http.HandleFunc("/api/v1/versions/", versions)
	http.HandleFunc("/api/v1/projects/root/", projectName)
	http.HandleFunc("/api/v1/projects/", dependency)
	http.HandleFunc("/", notFound)
	return http.ListenAndServe(addr, nil)
}
