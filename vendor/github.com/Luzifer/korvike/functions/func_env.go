package functions

import "os"

func init() {
	registerFunction("env", func(name string, v ...string) string {
		defaultValue := ""
		if len(v) > 0 {
			defaultValue = v[0]
		}
		if value, ok := os.LookupEnv(name); ok {
			return value
		}
		return defaultValue
	})
}
