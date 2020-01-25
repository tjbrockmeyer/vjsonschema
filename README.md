# vjsonschema
This project is a [json-schema](https://json-schema.org/) validator. 
It wraps and expands on an existing project [gojsonschema](https://github.com/xeipuuv/gojsonschema).

## Project Goals

The target of this project is to expose an API for:
  * loading many different schemas (potentially) using a variety of different methods
  * allow any loaded schema to reference any other loaded schema
  * allow validating an instance against any of the loaded schemas
  * continue to allow easy integration with Swagger/OpenAPI

To this end, gojsonschema was the closest, but not quite.
This project makes use of `gojsonschema.SchemaLoader` and 
specifically the ability of the `.AddSchema()` method to cache external schemas from some arbitrary address.

## Usage Information

The general flow is as follows:
  1. Create a `Builder`
  2. Add schemas to the builder in one of three ways:
     * `AddDir()` for adding a whole directory of `.json` jsonschema files
     * `AddFile()` for adding a single `.json` file from any location on disk
     * `AddSchema()` for adding a schema from in-memory
  3. Compile a `Validator` from the builder
  4. Use the validator to validate some json in `[]byte` form against any of the added schemas

View the [example](#example-usage) below for an illustration.

### Compliant References

In order to take advantage of the benefits, references should be created as in the following:
```json
{
  "$ref": "{MyRef}"
}
```
The `{` and `}` surrounding a reference's text within the string are what identify a reference as compliant with this package.
References that are compliant will be automatically linked with the schemas that they refer to.
These referred schemas will never be loaded again after initialization is complete.

Of course, schemas may still be referenced using the canonical format as described in `gojsonschema`'s documentation,
but these references will not be compliant with this package, and they will be loaded on-the-fly as needed.

### Naming Convention

Seeing as references like `{MyRef}` don't seem to refer to any particular file, or any particular type necessarily,
it is important to know how the schema names are formed.
When adding a schema by file name or directory name, 
the schema will be named by the base name of the file (not including the extension).
If there are any definitions for any schema, the definition can be accessed using the schema name, plus the definition name.

**Generally:**

For the root schema: `schemaName = prefix + baseName(file)`  
For definition schemas: `schemaName = prefix + baseName(file) + definitionName`, 
where definitions can continue to recurse, if desired.

**For example:**

Given a directory `mySchemas` that contains a file `Schema.json`, 
once loaded, the root schema of that file may be referenced using `"{Schema}"`.

If `Schema.json` contains a `definitions` key, and there exists a definition called `MyDefinition`,
then that definition in particular may be referenced using `"{SchemaMyDefinition}"`.

When loading schemas using directories or files, a prefix may be provided as well.
The prefix will come before the file name: `prefixSchema`.

### Additional Functionality

Because one of the goals of this project is to allow simple integration with Swagger/OpenAPI,
there are a couple of tools that are exposed for conversion needs.

The first is `Builder.GetSchemas()` which returns a map of schema names to the full json schemas as `[]byte`s.

The second is `SchemaRefReplace()` which can be used to replace all references inside a schema 
with a new value based on the original value. 
This function is also used internally to create canonical references to the virtual schemas.

Using these two functions, schemas can be pulled out, 
modified to use correct references to the specification's schema list, and then saved into the specification itself.

With Swagger 2, references typically look like `#/definitions/Name`

With OpenAPI 3, references typically look like `#/components/schemas/Name`

## Example Usage

`./schemas/MySchema.json`
```json
{
  "type": "object",
  "required": ["myDate", "myYear"],
  "properties": {
    "myDate": {"$ref": "{Date}"},
    "myYear": {"$ref": "{DateYear}"}
  }
}
```

`./otherSchemas/Date.json`
```json
{
  "type": "string",
  "format": "dateTime",
  "definitions": {
    "Year": {
      "type": "integer",
      "example": 2020
    }
  }
}
```

`./main.go`
```go
package main

import (
    "github.com/tjbrockmeyer/vjsonschema"
    "strings"
)

func main() {
    vb := vjsonschema.NewBuilder()
    
    if err := vb.AddDir("", "./schemas"); err != nil {
        panic(err)
    }
    if err := vb.AddFile("", "./otherSchemas/Date.json"); err != nil {
        panic(err)
    }
    if err := vb.AddSchema("MyObject", []byte(`{"type":"object","properties":{"x":{"$ref":"{MySchema}"}}}`)); err != nil {
        panic(err)
    }
    v, err := vb.Compile()
    if err != nil {
        panic(err)
    }
    instance := []byte(`{"x":{"myDate":"01/24/2020","myYear":2020}}`) 
    result, err := v.Validate("MyObject", instance)
    if err != nil {
        panic(err)
    }

    // result is a *gojsonschema.Result. View their documentation for more information.
    if !result.Valid() {
        errs := make([]string, 0, len(result.Errors()))
        for _, err := range result.Errors() {
            errs = append(errs, err.String())
        }
        err := "jsonschema validation error: \n\t" + strings.Join(errs, "\n\t")
        panic(err)
    }
}
```
