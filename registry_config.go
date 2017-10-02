package dep

import (
	"bytes"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
	"io"
)

// RegistryConfigName is the registry config file name used by dep.
const RegistryConfigName = "Gopkg.reg"

// registryConfig holds registry config file data and implements gps.Registry.
type registryConfig struct {
	url   string
	token string
}

// create new registry config using url and token for authentication
func NewRegistryConfig(url, token string) *registryConfig {
	return &registryConfig{url: url, token: token}
}

func (rc *registryConfig) URL() string {
	return rc.url
}

func (rc *registryConfig) Token() string {
	return rc.token
}

type rawConfig struct {
	Registry rawRegistry `toml:"registry"`
}

type rawRegistry struct {
	Url   string `toml:"url"`
	Token string `toml:"token"`
}

// readConfig returns a Registry read from r and a slice of validation warnings.
func readConfig(r io.Reader) (*registryConfig, error) {
	buf := &bytes.Buffer{}
	_, err := buf.ReadFrom(r)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to read byte stream")
	}
	raw := rawConfig{}
	err = toml.Unmarshal(buf.Bytes(), &raw)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse the registry config as TOML")
	}

	config := &registryConfig{}
	config.url = raw.Registry.Url
	config.token = raw.Registry.Token

	return config, err
}

// toRaw converts the registry config into a representation suitable to write to the registry config file
func (rc *registryConfig) toRaw() rawConfig {
	raw := rawConfig{
		Registry: rawRegistry{
			Url:   rc.url,
			Token: rc.token,
		},
	}
	return raw
}

// MarshalTOML serializes this registry config into TOML via an intermediate raw form.
func (rc *registryConfig) MarshalTOML() ([]byte, error) {
	raw := rc.toRaw()
	result, err := toml.Marshal(raw)
	return result, errors.Wrap(err, "Unable to marshal registry config to TOML string")
}
