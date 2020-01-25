package vjsonschema

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/xeipuuv/gojsonschema"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	refRegex = regexp.MustCompile(`"\$ref"\s*:\s*"{([^"]*?)}"`)
)

// Replaces all $ref values that are surrounded by { and } using the provided replacement function.
func SchemaRefReplace(schema []byte, replaceFunc func(ref string) string) []byte {
	return refRegex.ReplaceAllFunc(schema, func(match []byte) []byte {
		ref := refRegex.FindSubmatch(match)[1]
		return []byte(fmt.Sprintf(`"$ref":"%s"`, replaceFunc(string(ref))))
	})
}

type Builder interface {
	AddDir(prefix, dir string) error
	AddFile(prefix, filePath string) error
	AddSchema(name string, schema []byte) error
	GetSchemas() map[string][]byte
	Compile() (Validator, error)
}

type Validator interface {
	Validate(schemaName string, instance []byte) (*gojsonschema.Result, error)
}

type builder struct {
	schemas map[string]registeredSchema
}

type registeredSchema struct {
	source             []byte
	requiredReferences map[string]struct{}
}

func NewBuilder() Builder {
	return &builder{
		schemas: make(map[string]registeredSchema, 20),
	}
}

// Adds an entire directory (non-recursive) as schemas.
// Every .json file in 'dir' will be opened and added to the schema map as follows:
// 		For the main schema: prefix+fileName
// 		For the definitions: prefix+fileName+definitionName
func (v *builder) AddDir(prefix, dir string) error {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		name := info.Name()
		if strings.HasSuffix(name, ".json") {
			rootTypeName := name[:len(name)-5]
			return v.AddFile(prefix+rootTypeName, path)
		}
		return nil
	})
	if err != nil {
		return errors.WithMessage(err, "failed during directory walk of "+dir)
	}
	return nil
}

// Opens the file and adds it to the schema map as follows:
// 		For the main schema: prefix+fileName
// 		For the definitions: prefix+fileName+definitionName
func (v *builder) AddFile(prefix, filePath string) error {
	if filepath.Ext(filePath) != ".json" {
		return errors.New("failed to add file as schema - file must have a .json ext")
	}
	name := filepath.Base(filePath)
	name = prefix + name[:len(name)-5]
	if contents, err := ioutil.ReadFile(filePath); err != nil {
		return errors.WithMessage(err, "failed to read file: "+filePath)
	} else if err = v.AddSchema(name, contents); err != nil {
		return errors.WithMessage(err, "failed to add schema from file: "+filePath)
	}
	return nil
}

// Adds a schema to the schema map as 'name'
// 		For the main schema: name
// 		For the definitions: name+definitionName
func (v *builder) AddSchema(name string, schema []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(schema, &m); err != nil {
		return errors.WithMessage(err, "schema is not is correct json format")
	}
	return v.addSchema(name, m)
}

// Return a mapping of name to copies of the schemas.
func (v *builder) GetSchemas() map[string][]byte {
	out := make(map[string][]byte, len(v.schemas))
	for name, s := range v.schemas {
		out[name] = s.source
	}
	return out
}

// Compile all added schemas into a validator for any of the prefixs.
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
		if schema, err := loader.Compile(gojsonschema.NewBytesLoader(SchemaRefReplace(s.source, refToFakeExternal))); err != nil {
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
				defName := name + defKey
				if defMap, ok := def.(map[string]interface{}); !ok {
					return fmt.Errorf("expected definition for '%s' to be an object at path: %s", defKey, defName)
				} else if err := v.addSchema(defName, defMap); err != nil {
					return errors.WithMessage(err, "failed to add schema with name: "+defName)
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
	v.schemas[name] = registeredSchema{
		source:             b,
		requiredReferences: refs,
	}
	return nil
}

type validator struct {
	schemas map[string]*gojsonschema.Schema
}

// Validate that a particular json blob conforms to the given schema.
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
		if err := loader.AddSchema(refToFakeExternal(name), gojsonschema.NewBytesLoader(SchemaRefReplace(s.source, refToFakeExternal))); err != nil {
			return errors.WithMessage(err, "gojsonschema: failed to load schema with name: "+name)
		}
	}
	return nil
}

func refToFakeExternal(ref string) string {
	return "file://--jsonschema--/" + ref
}