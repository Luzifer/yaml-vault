package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v2"

	"github.com/hashicorp/vault/api"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	korvike "github.com/Luzifer/korvike/functions"
	"github.com/Luzifer/rconfig/v2"
)

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
		Verbose        bool     `flag:"verbose,v" default:"false" description:"Print verbose output [DEPRECATED]"`
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

	data, err := ioutil.ReadFile(vf)
	if err != nil {
		return ""
	}

	return string(data)
}

func init() {
	rconfig.SetVariableDefaults(map[string]string{
		"vault-token": vaultTokenFromDisk(),
	})
	if err := rconfig.ParseAndValidate(&cfg); err != nil {
		log.WithError(err).Fatal("Unable to parse commandline options")
	}

	if cfg.VersionAndExit {
		fmt.Printf("vault2env %s\n", version)
		os.Exit(0)
	}

	if l, err := log.ParseLevel(cfg.LogLevel); err != nil {
		log.WithError(err).Fatal("Unable to parse log level")
	} else {
		log.SetLevel(l)
	}

	if cfg.Verbose {
		// Backwards compatibility
		log.SetLevel(log.DebugLevel)
	}

	if cfg.Export == cfg.Import {
		log.Fatal("You need to either import or export")
	}

	if _, err := os.Stat(cfg.File); (err == nil && cfg.Export) || (err != nil && cfg.Import) {
		if cfg.Export {
			log.Fatal("Output file exists, stopping now.")
		}
		log.Fatal("Input file does not exist, stopping now.")
	}
}

func main() {
	client, err := api.NewClient(&api.Config{
		Address: cfg.VaultAddress,
	})
	if err != nil {
		log.WithError(err).Fatal("Unable to create client")
	}

	client.SetToken(cfg.VaultToken)

	var ex execFunction

	if cfg.Export {
		ex = exportFromVault
	} else {
		ex = importToVault
	}

	if err = ex(client); err != nil {
		log.WithError(err).Fatal("Unable to execute requested action")
	}
}

func exportFromVault(client *api.Client) error {
	out := importFile{}

	for _, path := range cfg.ExportPaths {
		if path[0] == '/' {
			path = path[1:]
		}

		if !strings.HasSuffix(path, "/") {
			path = path + "/"
		}

		if err := readRecurse(client, path, &out); err != nil {
			return errors.Wrap(err, "Unable to read from Vault")
		}
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		return errors.Wrap(err, "Unable to marshal yaml")
	}

	return ioutil.WriteFile(cfg.File, data, 0600)
}

func readRecurse(client *api.Client, path string, out *importFile) error {
	if !strings.HasSuffix(path, "/") {
		secret, err := client.Logical().Read(path)
		if err != nil {
			return errors.Wrapf(err, "Unable to read path %q", path)
		}

		if secret == nil {
			if cfg.IgnoreErrors {
				log.WithField("path", path).Info("Unable to read nil secret")
				return nil
			}
			return errors.Errorf("Unable to read non-existent path %s", path)
		}

		out.Keys = append(out.Keys, importField{Key: path, Values: secret.Data})
		log.WithField("path", path).Debug("Successfully read data from key")
		return nil
	}

	secret, err := client.Logical().List(path)
	if err != nil {
		if cfg.IgnoreErrors {
			log.WithError(err).WithField("path", path).Error("Error reading secret")
			return nil
		}
		return errors.Wrapf(err, "Error reading %s", path)
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

func importToVault(client *api.Client) error {
	keysRaw, err := ioutil.ReadFile(cfg.File)
	if err != nil {
		return errors.Wrap(err, "Unable to read input file")
	}

	keysRaw, err = parseImportFile(keysRaw)
	if err != nil {
		return errors.Wrap(err, "Unable to parse input file")
	}

	var keys importFile
	if err := yaml.Unmarshal(keysRaw, &keys); err != nil {
		return errors.Wrap(err, "Unable to unmarshal input file")
	}

	for _, field := range keys.Keys {
		if field.State == "absent" {
			if _, err := client.Logical().Delete(field.Key); err != nil {
				if cfg.IgnoreErrors {
					log.WithError(err).WithField("path", field.Key).Error("Error while deleting key")
					continue
				}
				return errors.Wrapf(err, "Unable to delete path %q", field.Key)
			}
			log.WithField("path", field.Key).Debug("Successfully deleted key")
		} else {
			if _, err := client.Logical().Write(field.Key, field.Values); err != nil {
				if cfg.IgnoreErrors {
					log.WithError(err).WithField("path", field.Key).Error("Error while writing data to key")
					continue
				}
				return errors.Wrapf(err, "Unable to write path %q", field.Key)
			}
			log.WithField("path", field.Key).Debug("Successfully wrote data to key")
		}
	}

	return nil
}

func parseImportFile(in []byte) (out []byte, err error) {
	t, err := template.New("input file").Funcs(korvike.GetFunctionMap()).Parse(string(in))
	if err != nil {
		return nil, errors.Wrap(err, "Unable to parse template")
	}

	buf := bytes.NewBuffer([]byte{})
	err = t.Execute(buf, nil)
	return buf.Bytes(), errors.Wrap(err, "Unable to execute template")
}
