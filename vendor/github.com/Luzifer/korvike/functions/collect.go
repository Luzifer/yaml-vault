package functions

import (
	"errors"
	"sync"
	"text/template"
)

var (
	templateFunctions     = template.FuncMap{}
	templateFunctionsLock sync.Mutex
)

func registerFunction(name string, f interface{}) error {
	templateFunctionsLock.Lock()
	defer templateFunctionsLock.Unlock()
	if _, ok := templateFunctions[name]; ok {
		return errors.New("Duplicate function for name " + name)
	}
	templateFunctions[name] = f
	return nil
}

// GetFunctionMap exports all functions used in korvike to be used in own projects
// Example:
//     import korvike "github.com/Luzifer/korvike"
//     tpl := template.New("mytemplate").Funcs(korvike.GetFunctionMap())
func GetFunctionMap() template.FuncMap {
	return templateFunctions
}
