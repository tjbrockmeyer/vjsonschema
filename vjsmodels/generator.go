package vjsmodels

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"go/format"
	"regexp"
	"sort"
	"strings"
)

const (
	none = iota
	isAcceptAll
	isAcceptNone
	isArray
	isObject
)

var (
	azRegex = regexp.MustCompile(`^[A-Za-z]`)
)

type field struct {
	name     string
	required bool
	schema   *jsonSchema
}

type jsonSchema struct {
	jsonSchemaBase

	nullable    bool
	goType      string
	specialType int
	fields      []field
}

type jsonSchemaBase struct {
	Title                string                 `json:"title"`
	Description          string                 `json:"description"`
	Ref                  string                 `json:"$ref"`
	Type                 interface{}            `json:"type"`
	Items                interface{}            `json:"items"`
	Properties           map[string]*jsonSchema `json:"properties"`
	Required             []string               `json:"required"`
	PatternProperties    map[string]*jsonSchema `json:"patternProperties"`
	AdditionalProperties *jsonSchema            `json:"additionalProperties"`
	OneOf                []*jsonSchema          `json:"oneOf"`
	AllOf                []*jsonSchema          `json:"allOf"`
	AnyOf                []*jsonSchema          `json:"anyOf"`
}

func (s *jsonSchema) UnmarshalJSON(b []byte) error {
	var k bool
	err := json.Unmarshal(b, &k)
	if err == nil {
		if k {
			s.goType = "interface{}"
			s.specialType = isAcceptAll
		} else {
			s.goType = "struct{}"
			s.specialType = isAcceptNone
		}
		return nil
	}
	err = json.Unmarshal(b, &s.jsonSchemaBase)
	if err != nil {
		return errors.WithMessage(err, "jsonschema must be one of {boolean, object}")
	}
	return nil
}

func (s *jsonSchema) handleOneAnyAllOf(ofSchemas []*jsonSchema, schemas map[string]*jsonSchema, required bool, keyName string) error {
	for i, s2 := range ofSchemas {
		if err := s2.getGoType(schemas, true); err != nil {
			return errors.WithMessagef(err, "schema '%v'", i)
		}
		if s2.specialType != isObject {
			s.goType = "interface{}"
			return nil
		}
	}
	s.specialType = isObject
	var builder strings.Builder
	builder.WriteString("struct{\n")
	for i, s2 := range ofSchemas {
		var ref string
		if s2.Ref != "" {
			ref, _ = isCompliantRef(s2.Ref)
		}
		builder.WriteString(fmt.Sprintf("// %s: schema #%v\n", keyName, i))
		if _, ok := schemas[ref]; ok {
			builder.WriteString("*" + toIdentifier(ref) + "\n")
		} else {
			t := strings.Split(s2.goType, "\n")
			builder.WriteString(strings.Join(t[1:len(t)-1], "\n") + "\n")
		}
	}
	builder.WriteString("}")
	s.goType = builder.String()
	return nil
}

func (s *jsonSchema) handleArray(schemas map[string]*jsonSchema, required bool) error {
	s.specialType = isArray
	if _, ok := s.Items.([]interface{}); ok {
		b, _ := json.Marshal(s.Items)
		var oneOf []*jsonSchema
		if err := json.Unmarshal(b, &oneOf); err != nil {
			return errors.New("keyword 'items' should be one of {schema, []schema}")
		}
		if err := s.handleOneAnyAllOf(oneOf, schemas, required, "oneOf"); err != nil {
			return errors.WithMessage(err, "keyword 'items'")
		}
		s.goType = "[]" + s.goType
		return nil
	} else {
		b, _ := json.Marshal(s.Items)
		s2 := new(jsonSchema)
		if err := json.Unmarshal(b, s2); err != nil {
			return errors.New("keyword 'items' should be one of {schema, []schema}")
		}
		err := s2.getGoType(schemas, true)
		s.goType = "[]" + s2.goType
		return errors.WithMessage(err, "keyword 'items'")
	}
}

func (s *jsonSchema) handleObject(schemas map[string]*jsonSchema, required bool) error {
	if s.AdditionalProperties != nil {
		if s.AdditionalProperties.specialType == isAcceptAll {
			s.goType = "map[string]interface{}"
			return nil
		} else if s.AdditionalProperties.specialType != isAcceptNone {
			if err := s.AdditionalProperties.getGoType(schemas, true); err != nil {
				return errors.WithMessage(err, "keyword 'additionalProperties'")
			}
			s.goType = "map[string]" + s.AdditionalProperties.goType
			return nil
		}
		// if additionalProperties = false, just treat it like a normal object.
	}

	if s.PatternProperties != nil {
		if len(s.PatternProperties) == 1 {
			if err := s.getGoType(schemas, true); err != nil {
				return errors.WithMessage(err, "keyword 'patternProperties'")
			}
			s.goType = "map[string]" + s.goType
			return nil
		} else if len(s.PatternProperties) > 1 {
			s.goType = "map[string]interface{}"
		}
		// if patternProperties = {}, just treat it like a normal object.
	}

	s.specialType = isObject

	reqList := make(map[string]struct{})
	if s.Required != nil {
		for _, req := range s.Required {
			reqList[req] = struct{}{}
		}
	}

	if s.Properties != nil {
		var b strings.Builder
		b.WriteString("struct{")
		props := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			props = append(props, name)
		}
		sort.Strings(props)
		for _, name := range props {
			schema := s.Properties[name]
			_, isRequired := reqList[name]
			if err := schema.getGoType(schemas, isRequired); err != nil {
				return errors.WithMessage(err, "keyword 'properties."+name+"'")
			}
			var omitEmpty string
			if !isRequired {
				omitEmpty = ",omitempty"
			}
			b.WriteString(fmt.Sprintf("\n%s %s `json:\"%s%s\"`", toIdentifier(name), schema.goType, name, omitEmpty))
		}
		b.WriteString("\n}")
		s.goType = b.String()
		return nil
	}
	s.goType = "map[string]interface{}"
	return nil
}

func (s *jsonSchema) handleType(kwType interface{}, schemas map[string]*jsonSchema, required bool) error {
	if t, ok := kwType.(string); ok {
		switch t {
		case "null":
			s.goType = "struct{}"
		case "boolean":
			s.goType = "bool"
		case "integer":
			s.goType = "int"
		case "number":
			s.goType = "float64"
		case "string":
			s.goType = "string"
		case "array":
			return s.handleArray(schemas, required)
		case "object":
			return s.handleObject(schemas, required)
		default:
			return errors.New("valid values for keyword 'type' are {null, boolean, integer, number, string, array, object}")
		}
		return nil
	} else if l, ok := kwType.([]interface{}); ok {
		if len(l) == 1 {
			return s.handleType(l[0], schemas, required)
		}
		var types = make(map[string]struct{})
		for _, t := range l {
			if t == "null" {
				s.nullable = true
			} else {
				ts, ok := t.(string)
				if !ok {
					return errors.New("valid types for keyword 'type' are {string, []string}")
				}
				types[ts] = struct{}{}
				if len(types) > 1 {
					s.goType = "interface{}"
					return nil
				}
			}
		}
		for t := range types {
			return s.handleType(t, schemas, required)
		}
		s.goType = "struct{}"
		return nil
	} else {
		return errors.New("valid types for keyword 'type' are {string, []string}")
	}
}

func (s *jsonSchema) getGoType(schemas map[string]*jsonSchema, required bool) error {
	if s.Ref != "" {
		if s2Name, ok := isCompliantRef(s.Ref); ok {
			s2, ok := schemas[s2Name]
			if !ok {
				return fmt.Errorf("schema \"%s\" is not defined", s2Name)
			}
			if err := s2.getGoType(schemas, required); err != nil {
				return errors.WithMessage(err, "keyword '$ref'")
			}
			s.specialType = s2.specialType
			if s2.canBeReferenced() {
				s.goType = toIdentifier(s2Name)
				return nil
			}
			s.goType = s2.goType
			return nil
		}
		s.goType = "interface{}"
		return nil
	}

	if s.AnyOf != nil {
		return errors.WithMessage(s.handleOneAnyAllOf(s.AnyOf, schemas, required, "anyOf"), "keyword 'anyOf'")
	}
	if s.OneOf != nil {
		return errors.WithMessage(s.handleOneAnyAllOf(s.OneOf, schemas, required, "oneOf"), "keyword 'oneOf'")
	}
	if s.AllOf != nil {
		return errors.WithMessage(s.handleOneAnyAllOf(s.AllOf, schemas, required, "allOf"), "keyword 'allOf'")
	}
	if s.Type != nil {
		return errors.WithMessage(s.handleType(s.Type, schemas, required), "keyword 'type'")
	}
	s.goType = "interface{}"
	return nil
}

func Generate(packageName string, schemas map[string][]byte) ([]byte, error) {
	var b bytes.Buffer

	jsonSchemas := make(map[string]*jsonSchema, len(schemas))
	for name, schema := range schemas {
		s := new(jsonSchema)
		if err := json.Unmarshal(schema, s); err != nil {
			return nil, errors.WithMessage(err, "failed to marshal schema into json "+name)
		}
		jsonSchemas[name] = s
	}

	b.WriteString("package " + packageName + "\n")

	names := make([]string, 0, len(jsonSchemas))
	for name := range jsonSchemas {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := jsonSchemas[name]
		if err := s.getGoType(jsonSchemas, true); err != nil {
			return nil, errors.WithMessage(err, "failed to get type of schema "+name)
		}
		if s.Title != "" {
			b.WriteString("// " + s.Title + "\n")
		}
		if s.Description != "" {
			b.WriteString("// " + s.Description + "\n")
		}
		if !s.canBeReferenced() {
			b.WriteString("// ")
		}
		b.WriteString(fmt.Sprintf("type %s %s\n\n", name, s.goType))
	}

	if src, err := format.Source(b.Bytes()); err != nil {
		return src, errors.WithMessage(err, "failed to parse models as go source")
	} else {
		return src, nil
	}
}

func (s *jsonSchema) canBeReferenced() bool {
	return s.specialType == isObject || s.specialType == isArray
}

func toIdentifier(s string) string {
	if len(s) == 0 {
		return "X"
	}
	if m := azRegex.MatchString(s); !m {
		return "X" + s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func isCompliantRef(ref string) (r string, ok bool) {
	if strings.HasPrefix(ref, "{") && strings.HasSuffix(ref, "}") {
		return ref[1 : len(ref)-1], true
	}
	return "", false
}
