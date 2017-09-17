package registry

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"os"
	"errors"
	"path/filepath"
	"encoding/hex"
)

const TOKEN_AUTH = "2erygdasE45rty5JKwewrr75cb15rdeE"

var tokenResp = "{\"token\": \"" + TOKEN_AUTH + "\"}"

type rawVersions struct {
	Versions map[string]rawPublished `json:"versions"`
}
type rawPublished struct {
	Published string `json:"published"`
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
	return filepath.Join(wd, "testdata", "registry", "sources"), nil
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
		fmt.Println("StatusUnauthorized")
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
		fmt.Println("Error", r.Method)
		http.NotFound(w, r)
	}
}

func notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func SetupAndRun(addr string) error {
	http.HandleFunc("/api/v1/auth/token", token)
	http.HandleFunc("/api/v1/versions/", versions)
	http.HandleFunc("/api/v1/projects/", dependency)
	http.HandleFunc("/", notFound)
	return http.ListenAndServe(addr, nil)
}
