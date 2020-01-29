package vjsonschema

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// An object that is capable of building a Validator from schemas.
type Builder interface {
	// Adds an entire directory (non-recursive) as schemas.
	// Every .json file in 'dir' will be opened and added to the schema map.
	// Root schemas will be added to the map under the file name.
	// Definitions are added to the map under their respective names.
	AddDir(dir string) error

	// Opens the file and adds it to the schema map.
	// The root schema will be added to the map under the file name.
	// Definitions are added to the map under their respective names.
	AddFile(filePath string) error

	// Adds a schema to the schema map as 'name'
	// Definitions are added to the map under their respective names.
	AddSchema(name string, schema interface{}) error

	// Return a mapping of name to copies of the schemas.
	GetSchemas() map[string][]byte

	// Compile all added schemas into a validator for any of the prefixs.
	Compile() (Validator, error)
}

type builder struct {
	schemas map[string]registeredSchema
}

type registeredSchema struct {
	source             []byte
	requiredReferences map[string]struct{}
}

// Get a new bulider for creating a validator.
func NewBuilder() Builder {
	return &builder{
		schemas: make(map[string]registeredSchema, 20),
	}
}

func (v *builder) AddDir(dir string) error {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		name := info.Name()
		if strings.HasSuffix(name, ".json") {
			return v.AddFile(path)
		}
		return nil
	})
	if err != nil {
		return errors.WithMessage(err, "failed during directory walk of "+dir)
	}
	return nil
}

func (v *builder) AddFile(filePath string) error {
	if filepath.Ext(filePath) != ".json" {
		return errors.New("failed to add file as schema - file must have a .json ext")
	}
	name := filepath.Base(filePath)
	name = name[:len(name)-5]
	if contents, err := ioutil.ReadFile(filePath); err != nil {
		return errors.WithMessage(err, "failed to read file: "+filePath)
	} else if err = v.AddSchema(name, contents); err != nil {
		return errors.WithMessage(err, "failed to add schema from file: "+filePath)
	}
	return nil
}

func (v *builder) AddSchema(name string, schema interface{}) error {
	var (
		b   json.RawMessage
		err error
	)
	if _, ok := schema.(json.RawMessage); !ok {
		if k, ok := schema.([]byte); ok {
			b = k
		} else if k, ok := schema.(string); ok {
			b = json.RawMessage(k)
		} else {
			b, err = json.Marshal(schema)
			if err != nil {
				return errors.WithMessage(err, "failed to marshal schema as json")
			}
		}
	}
	var m map[string]interface{}
	if err = json.Unmarshal(b, &m); err != nil {
		return errors.WithMessage(err, "schema must be a correctly formatted json object")
	}
	return v.addSchema(name, m)
}

func (v *builder) GetSchemas() map[string][]byte {
	out := make(map[string][]byte, len(v.schemas))
	for name, s := range v.schemas {
		out[name] = s.source
	}
	return out
}

func (v *builder) Compile() (Validator, error) {
	schemas := make(map[string]*gojsonschema.Schema, len(v.schemas))

	missingRefs := make(map[string]struct{})
	for name, s := range v.schemas {
		for n := range s.requiredReferences {
			if _, ok := v.schemas[n]; !ok {
				missingRefs[n+"("+name+")"] = struct{}{}
			}
		}
	}
	if len(missingRefs) > 0 {
		x := make([]string, 0, len(missingRefs))
		for r := range missingRefs {
			x = append(x, r)
		}
		return nil, errors.New("missing required references: " + strings.Join(x, ", "))
	}

	for name, s := range v.schemas {
		loader := gojsonschema.NewSchemaLoader()
		schemasAdded := make(map[string]bool, 7)
		schemasAdded[name] = false
		for n := range s.requiredReferences {
			if err := addSchemasCompile(v.schemas, &schemasAdded, loader, n); err != nil {
				return nil, err
			}
		}
		if schema, err := loader.Compile(gojsonschema.NewBytesLoader(SchemaRefReplace(s.source, refNameConvert))); err != nil {
			return nil, errors.WithMessage(err, "gojsonschema: failed to compile schema with name: "+name)
		} else {
			schemas[name] = schema
		}
	}

	return &validator{schemas: schemas}, nil
}

func (v *builder) addSchema(name string, schema map[string]interface{}) error {
	if defs, ok := schema["definitions"]; ok {
		if defsMap, ok := defs.(map[string]interface{}); !ok {
			return errors.New("expected 'definitions' key of schema to be an object")
		} else {
			for defKey, def := range defsMap {
				if defMap, ok := def.(map[string]interface{}); !ok {
					return fmt.Errorf("expected definition for '%s' to be an object", defKey)
				} else if err := v.addSchema(defKey, defMap); err != nil {
					return errors.WithMessage(err, "failed to add schema with name: "+defKey)
				}
			}
		}
	}
	delete(schema, "definitions")
	b, _ := json.Marshal(schema)
	items := refRegex.FindAllSubmatch(b, -1)
	refs := make(map[string]struct{}, len(items))
	for _, i := range items {
		refs[string(i[1])] = struct{}{}
	}
	if _, ok := v.schemas[name]; ok {
		return errors.New("multiple definitions for schema with name: " + name)
	}
	v.schemas[name] = registeredSchema{
		source:             b,
		requiredReferences: refs,
	}
	return nil
}
