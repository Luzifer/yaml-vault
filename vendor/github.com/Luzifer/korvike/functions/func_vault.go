package functions

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/hashicorp/vault/api"
	homedir "github.com/mitchellh/go-homedir"
)

func init() {
	registerFunction("vault", func(name string, v ...string) (interface{}, error) {
		if name == "" {
			return nil, fmt.Errorf("Path is not set")
		}
		if len(v) < 1 {
			return nil, fmt.Errorf("Key is not set")
		}

		client, err := api.NewClient(&api.Config{
			Address: os.Getenv(api.EnvVaultAddress),
		})
		if err != nil {
			return nil, err
		}

		client.SetToken(vaultTokenFromEnvOrFile())

		secret, err := client.Logical().Read(name)
		if err != nil {
			return nil, err
		}

		if secret != nil && secret.Data != nil {
			if val, ok := secret.Data[v[0]]; ok {
				return val, nil
			}
		}

		if len(v) < 2 {
			return nil, fmt.Errorf("Requested value %q in key %q was not found in Vault and no default was set", v[0], name)
		}

		return v[1], nil
	})
}

func vaultTokenFromEnvOrFile() string {
	if token := os.Getenv(api.EnvVaultToken); token != "" {
		return token
	}

	if f, err := homedir.Expand("~/.vault-token"); err == nil {
		if b, err := ioutil.ReadFile(f); err == nil {
			return string(b)
		}
	}

	return ""
}
