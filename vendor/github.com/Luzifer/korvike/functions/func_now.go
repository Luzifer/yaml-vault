package functions

import "time"

func init() {
	registerFunction("now", func(name string, v ...string) string {
		return time.Now().Format(name)
	})
}
