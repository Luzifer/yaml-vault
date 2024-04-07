package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/hashicorp/vault/api"
	"github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"

	korvike "github.com/Luzifer/korvike/functions"
	"github.com/Luzifer/rconfig/v2"
)

const filePermissionUserWrite = 0o600

var (
	cfg = struct {
		File           string   `flag:"file,f" default:"vault.yaml" description:"File to import from / export to" validate:"nonzero"`
		Import         bool     `flag:"import" default:"false" description:"Enable importing data into Vault"`
		Export         bool     `flag:"export" default:"false" description:"Enable exporting data from Vault"`
		ExportPaths    []string `flag:"export-paths" default:"secret" description:"Which paths to export"`
		IgnoreErrors   bool     `flag:"ignore-errors" default:"false" description:"Do not exit on read/write errors"`
		LogLevel       string   `flag:"log-level" default:"info" description:"Log level (debug, info, warn, error, fatal)"`
		VaultAddress   string   `flag:"vault-addr" env:"VAULT_ADDR" default:"https://127.0.0.1:8200" description:"Vault API address"`
		VaultToken     string   `flag:"vault-token" env:"VAULT_TOKEN" vardefault:"vault-token" description:"Specify a token to use instead of app-id auth" validate:"nonzero"`
		VersionAndExit bool     `flag:"version" default:"false" description:"Print program version and exit"`
	}{}

	version = "dev"
)

type importFile struct {
	Keys []importField
}

type importField struct {
	Key    string
	State  string
	Values map[string]interface{}
}

type execFunction func(*api.Client) error

func vaultTokenFromDisk() string {
	vf, err := homedir.Expand("~/.vault-token")
	if err != nil {
		return ""
	}

	data, err := os.ReadFile(vf) //#nosec G304 // Intended to read file from disk
	if err != nil {
		return ""
	}

	return string(data)
}

func initApp() error {
	rconfig.AutoEnv(true)
	rconfig.SetVariableDefaults(map[string]string{
		"vault-token": vaultTokenFromDisk(),
	})
	if err := rconfig.ParseAndValidate(&cfg); err != nil {
		return fmt.Errorf("parsing CLI options: %w", err)
	}

	l, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("parsing log-level: %w", err)
	}
	logrus.SetLevel(l)

	return nil
}

func main() {
	var err error
	if err = initApp(); err != nil {
		logrus.WithError(err).Fatal("initializing app")
	}

	if cfg.VersionAndExit {
		fmt.Printf("yaml-vault %s\n", version) //nolint:forbidigo
		os.Exit(0)
	}

	if cfg.Export == cfg.Import {
		logrus.Fatal("either import or export must be set")
	}

	if _, err := os.Stat(cfg.File); (err == nil && cfg.Export) || (err != nil && cfg.Import) {
		if cfg.Export {
			logrus.Fatal("output file exists, stopping now.")
		}
		logrus.Fatal("input file does not exist, stopping now.")
	}

	client, err := api.NewClient(&api.Config{
		Address: cfg.VaultAddress,
	})
	if err != nil {
		logrus.WithError(err).Fatal("creating Vault client")
	}

	client.SetToken(cfg.VaultToken)

	var ex execFunction
	if cfg.Export {
		ex = exportFromVault
	} else {
		ex = importToVault
	}

	if err = ex(client); err != nil {
		logrus.WithError(err).Fatal("executing requested action")
	}
}

func exportFromVault(client *api.Client) error {
	out := importFile{}

	for _, path := range cfg.ExportPaths {
		if path[0] == '/' {
			path = path[1:]
		}

		if !strings.HasSuffix(path, "/") {
			path += "/"
		}

		if err := readRecurse(client, path, &out); err != nil {
			return fmt.Errorf("reading from Vault: %w", err)
		}
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshalling YAML: %w", err)
	}

	if err = os.WriteFile(cfg.File, data, filePermissionUserWrite); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

func importToVault(client *api.Client) error {
	keysRaw, err := os.ReadFile(cfg.File)
	if err != nil {
		return fmt.Errorf("reading input-file: %w", err)
	}

	keysRaw, err = parseImportFile(keysRaw)
	if err != nil {
		return fmt.Errorf("parsing input file: %w", err)
	}

	var keys importFile
	if err := yaml.Unmarshal(keysRaw, &keys); err != nil {
		return fmt.Errorf("unmarshalling input file: %w", err)
	}

	for _, field := range keys.Keys {
		logger := logrus.WithField("path", field.Key)

		if field.State == "absent" {
			if _, err := client.Logical().Delete(field.Key); err != nil {
				if cfg.IgnoreErrors {
					logger.WithError(err).Error("deleting key")
					continue
				}
				return fmt.Errorf("deleting path %q: %w", field.Key, err)
			}
			logger.Debug("deleted key")
		} else {
			if _, err := client.Logical().Write(field.Key, field.Values); err != nil {
				if cfg.IgnoreErrors {
					logger.WithError(err).Error("writing data to key")
					continue
				}
				return fmt.Errorf("writing path %q: %w", field.Key, err)
			}
			logger.Debug("wrote data to key")
		}
	}

	return nil
}

func parseImportFile(in []byte) (out []byte, err error) {
	t, err := template.New("input file").Funcs(korvike.GetFunctionMap()).Parse(string(in))
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	buf := new(bytes.Buffer)
	if err = t.Execute(buf, nil); err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}

	return buf.Bytes(), nil
}

func readRecurse(client *api.Client, path string, out *importFile) error {
	if !strings.HasSuffix(path, "/") {
		secret, err := client.Logical().Read(path)
		if err != nil {
			return fmt.Errorf("reading path %q: %w", path, err)
		}

		if secret == nil {
			if cfg.IgnoreErrors {
				logrus.WithField("path", path).Info("read nil secret")
				return nil
			}
			return fmt.Errorf("read non-existent path %q", path)
		}

		out.Keys = append(out.Keys, importField{Key: path, Values: secret.Data})
		logrus.WithField("path", path).Debug("read data from key")
		return nil
	}

	secret, err := client.Logical().List(path)
	if err != nil {
		if cfg.IgnoreErrors {
			logrus.WithError(err).WithField("path", path).Error("reading secret")
			return nil
		}
		return fmt.Errorf("reading path %q: %w", path, err)
	}

	if secret != nil && secret.Data["keys"] != nil {
		for _, k := range secret.Data["keys"].([]interface{}) {
			if err := readRecurse(client, path+k.(string), out); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}
