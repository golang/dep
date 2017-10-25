package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/golang/dep"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"syscall"
	"path/filepath"
)

const loginShortHelp = `Login to a registry server and save configuration`
const loginLongHelp = `
Login to a remote registry server containing go dependencies and persist the login configuration.

If your registry allows anonymous access you can leave out the username and password parameters.
`

const noValue = "depNoInputProvided"

func (cmd *loginCommand) Name() string      { return "login" }
func (cmd *loginCommand) Args() string      { return "<url>" }
func (cmd *loginCommand) ShortHelp() string { return loginShortHelp }
func (cmd *loginCommand) LongHelp() string  { return loginLongHelp }
func (cmd *loginCommand) Hidden() bool      { return false }

func (cmd *loginCommand) Register(fs *flag.FlagSet) {
	fs.StringVar(&cmd.user, "u", noValue, "provide username for registry")
	fs.StringVar(&cmd.password, "p", noValue, "provide password for registry")
}

type loginCommand struct {
	user     string
	password string
}

func (cmd *loginCommand) getProject(ctx *dep.Ctx) (*dep.Project, error) {
	p, err := ctx.LoadProject()
	if p != nil {
		return p, err
	}
	p = new(dep.Project)
	if err := p.SetRoot(ctx.WorkingDir); err != nil {
		return nil, errors.Wrap(err, "NewProject")
	}
	return p, nil
}

func (cmd *loginCommand) Run(ctx *dep.Ctx, args []string) error {
	if len(args) > 1 {
		return errors.Errorf("too many args (%d)", len(args))
	}

	if len(args) != 1 {
		return errors.New("registry URL is required")
	}

	p, err := cmd.getProject(ctx)
	u, err := url.Parse(args[0])
	if err != nil {
		return err
	}
	u.Path = path.Join(u.Path, "api/v1/auth/token")

	var token string
	token, err = getToken(u.String(), cmd.user, cmd.password)
	if err != nil {
		return err
	}

	p.RegistryConfig = dep.NewRegistryConfig(args[0], token)
	rc, err := p.RegistryConfig.MarshalTOML()
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(filepath.Join(p.AbsRoot, dep.RegistryConfigName), rc, 0666); err != nil {
		return errors.Wrap(err, "write of registry configuration")
	}

	return nil
}

func readUsername() (string, error) {
	var user string
	print("username: ")
	_, err := fmt.Scanln(&user)
	if err != nil {
		return "", err
	}
	return user, nil
}

func readPassword() (string, error) {
	print("password: ")
	password, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	println()
	return string(password), nil
}

func getToken(url, user, password string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	if user == noValue {
		user, err = readUsername()
		if err != nil {
			return "", err
		}
	}
	if password == noValue {
		password, err = readPassword()
		if err != nil {
			return "", err
		}
	}

	req.SetBasicAuth(user, string(password))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("%s %s", url, http.StatusText(resp.StatusCode))
	}
	var bytes []byte
	bytes, err = ioutil.ReadAll(resp.Body)

	var loginResp rawLoginResp
	err = json.Unmarshal(bytes, &loginResp)
	return loginResp.Token, err
}

type rawLoginResp struct {
	Token string `json:"token"`
}
