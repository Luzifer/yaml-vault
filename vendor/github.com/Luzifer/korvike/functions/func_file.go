package functions

import (
	"io/ioutil"
	"os"
)

func init() {
	registerFunction("file", func(name string, v ...string) string {
		defaultValue := ""
		if len(v) > 0 {
			defaultValue = v[0]
		}
		if _, err := os.Stat(name); err == nil {
			if rawValue, err := ioutil.ReadFile(name); err == nil {
				return string(rawValue)
			}
		}
		return defaultValue
	})
}
