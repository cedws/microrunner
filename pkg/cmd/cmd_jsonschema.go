package cmd

import (
	"encoding/json"
	"log"
	"os"

	"github.com/swaggest/jsonschema-go"
	"go.cedwards.xyz/microrunner/pkg/microrunner"
)

type jsonschemaCmd struct{}

func (j *jsonschemaCmd) Run() error {
	reflector := jsonschema.Reflector{}
	schema, err := reflector.Reflect(microrunner.Config{})
	if err != nil {
		log.Fatal(err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	return enc.Encode(schema)
}
