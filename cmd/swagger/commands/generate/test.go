package generate

import (
	"fmt"
	"github.com/go-swagger/go-swagger/generator"
)



// Server the command to generate an entire server application
type Test struct {
	shared
	Name           string   `long:"name" short:"A" description:"the name of the application, defaults to a mangled value of info.title"`
	Operations     []string `long:"operation" short:"O" description:"specify an operation to include, repeat for multiple"`
	Tags           []string `long:"tags" description:"the tags to include, if not specified defaults to all"`
	Principal      string   `long:"principal" short:"P" description:"the model to use for the security principal"`
	Models         []string `long:"model" short:"M" description:"specify a model to include, repeat for multiple"`
	SkipModels     bool     `long:"skip-models" description:"no models will be generated when this flag is specified"`
	SkipOperations bool     `long:"skip-operations" description:"no operations will be generated when this flag is specified"`
	SkipSupport    bool     `long:"skip-support" description:"no supporting files will be generated when this flag is specified"`
	IncludeUI      bool     `long:"with-ui" description:"when generating a main package it uses a middleware that also serves a swagger-ui for the swagger json"`
	IncludeTCK     bool     `long:"with-tck" description:"when generating tests it generates a TCK compliance report"`

}

// Execute runs this command
func (t *Test) Execute(args []string) error {
	opts := generator.GenOpts{
		Spec:          string(t.Spec),
		Target:        string(t.Target),
		APIPackage:    t.APIPackage,
		ModelPackage:  t.ModelPackage,
		ServerPackage: t.ServerPackage,
		ClientPackage: t.ClientPackage,
		TestPackage:   t.TestPackage,
		Principal:     t.Principal,
	}



	if t.IncludeTCK {
		fmt.Println("======>  Adding TCK To TEST")
		if err := generator.GenerateTestSupport(t.Name, t.Models, t.Operations, t.IncludeUI, true, opts); err != nil {
			return err
		}
	}

	return nil
}
