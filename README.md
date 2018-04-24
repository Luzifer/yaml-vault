[![Go Report Card](https://goreportcard.com/badge/github.com/Luzifer/yaml-vault)](https://goreportcard.com/report/github.com/Luzifer/yaml-vault)
![](https://badges.fyi/github/license/Luzifer/yaml-vault)
![](https://badges.fyi/github/downloads/Luzifer/yaml-vault)
![](https://badges.fyi/github/latest-release/Luzifer/yaml-vault)

# Luzifer / yaml-vault

`yaml-vault` is a small utility to import data from a YAML file to Vault or export keys from Vault into a YAML file.

## Usage

```bash
# cat vault.yaml
keys:
  - key: secret/integration/test
    values:
      bar: foo
      foo: bar

# yaml-vault --import -f vault.yaml

# vault read secret/integration/test
Key                     Value
---                     -----
refresh_interval        2592000
bar                     foo
foo                     bar

```
