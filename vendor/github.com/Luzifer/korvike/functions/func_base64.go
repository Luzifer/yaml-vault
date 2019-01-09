package functions

import "encoding/base64"

func init() {
	registerFunction("b64encode", func(name string, v ...string) string {
		return base64.StdEncoding.EncodeToString([]byte(name))
	})
}
