package commands

import "github.com/go-swagger/go-swagger/cmd/swagger/commands/generate"

// Generate command to group all generator commands together
type Generate struct {
	Model     *generate.Model     `command:"model"`
	Operation *generate.Operation `command:"operation"`
	Support   *generate.Support   `command:"support"`
	Server    *generate.Server    `command:"server"`
	Test      *generate.Test      `command:"test"`
	Spec      *generate.SpecFile  `command:"spec"`
}
