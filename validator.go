// Package vjsonschema is a json-schema validator.
// This package wraps gojsonschema, exposing an API which allows for many virtual schemas to be defined,
// any of which may be validated against.
// This package also provides relatively simple integration with a Swagger/OpenAPI specification.
// View the full documentation on the github page: https://github.com/tjbrockmeyer/vjsonschema
package vjsonschema

import (
	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
	"log"
)

// An object that is capable of validating json against schemas.
type Validator interface {
	// Validate that a particular json blob conforms to the given schema.
	Validate(schemaName string, instance []byte) (*gojsonschema.Result, error)
}

type validator struct {
	schemas map[string]*gojsonschema.Schema
}

func (v *validator) Validate(schemaName string, instance []byte) (*gojsonschema.Result, error) {
	if schema, ok := v.schemas[schemaName]; !ok {
		return nil, errors.New("schema does not exist with name: " + schemaName)
	} else {
		log.Println(schemaName)
		log.Println(string(instance))
		return schema.Validate(gojsonschema.NewBytesLoader(instance))
	}
}

func addSchemasCompile(schemas map[string]registeredSchema, schemasAdded *map[string]bool, loader *gojsonschema.SchemaLoader, name string) error {
	s := schemas[name]
	for reqRef := range s.requiredReferences {
		if _, ok := (*schemasAdded)[reqRef]; !ok {
			(*schemasAdded)[reqRef] = false
			if err := addSchemasCompile(schemas, schemasAdded, loader, reqRef); err != nil {
				return err
			}
		}
	}
	if !(*schemasAdded)[name] {
		(*schemasAdded)[name] = true
		if err := loader.AddSchema(refNameConvert(name), gojsonschema.NewBytesLoader(SchemaRefReplace(s.source, refNameConvert))); err != nil {
			return errors.WithMessage(err, "gojsonschema: failed to load schema with name: "+name)
		}
	}
	return nil
}

func refNameConvert(ref string) string {
	return "file://--jsonschema--/" + ref
}
