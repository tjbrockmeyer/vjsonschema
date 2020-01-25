package vjsonschema_test

import (
	"github.com/tjbrockmeyer/vjsonschema"
	"io/ioutil"
	"strings"
	"testing"
)

func readFile(name string) []byte {
	contents, err := ioutil.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return contents
}

func testSchema(testName, passingName, failingName string) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()
		factory := vjsonschema.NewBuilder()
		if err := factory.AddFile("", "./schemas/"+testName+".json"); err != nil {
			t.Error(err)
		}
		for n := range factory.GetSchemas() {
			t.Log(n)
		}
		v, err := factory.Compile()
		if err != nil {
			t.Error(err)
		} else {
			r, err := v.Validate(passingName, readFile("./payloads/"+testName+"Pass.json"))
			if err != nil {
				t.Error(err)
			} else if !r.Valid() {
				for _, err := range r.Errors() {
					t.Error(err)
				}
			}
			r, err = v.Validate(failingName, readFile("./payloads/"+testName+"Fail.json"))
			if err != nil {
				t.Error(err)
			} else if r.Valid() {
				t.Error("expected test to fail")
			} else {
				for _, err := range r.Errors() {
					t.Log(err)
				}
			}
		}
	}
}

func TestValidator(t *testing.T) {
	t.Run("simple", testSchema("Simple", "Simple", "SimpleAbc"))
	t.Run("hasRefs", testSchema("HasRefs", "HasRefs", "HasRefsOne"))
	t.Run("circular", testSchema("Circular", "Circular", "Circular"))
	t.Run("multiple file refs", func(t *testing.T) {
		factory := vjsonschema.NewBuilder()
		if err := factory.AddFile("", "./schemas/F1.json"); err != nil {
			t.Error(err)
		}
		if err := factory.AddFile("", "./schemas/F2.json"); err != nil {
			t.Error(err)
		}
		for n := range factory.GetSchemas() {
			t.Log(n)
		}
		validator, err := factory.Compile()
		if err != nil {
			t.Error(err)
			return
		}
		f, err := ioutil.ReadFile("./payloads/f1f2Pass.json")
		if err != nil {
			t.Error(err)
			return
		}
		result, err := validator.Validate("F2", f)
		if err != nil {
			t.Error(err)
			return
		}
		if !result.Valid() {
			for _, err := range result.Errors() {
				t.Error(err)
			}
			return
		}
	})
	t.Run("missing refs", func(t *testing.T) {
		fac := vjsonschema.NewBuilder()
		if err := fac.AddFile("", "./schemas/MissingRefs.json"); err != nil {
			t.Error(err)
		}
		_, err := fac.Compile()
		if err == nil {
			t.Error("expected an error, found none")
		} else if !strings.Contains(err.Error(), "missing required references") {
			t.Error("found error, but it should be about missing references:", err)
		}
	})
}
