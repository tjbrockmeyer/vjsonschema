package main

import (
	"github.com/pkg/errors"
	"github.com/tjbrockmeyer/vjsonschema"
	"github.com/tjbrockmeyer/vjsonschema/vjsmodels"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
)

var (
	outfile = kingpin.Arg("output", "name of the file to write models to").Required().String()
	pkg     = kingpin.Arg("package", "name of the package to use for the generated models").Required().String()
	dirs    = kingpin.Flag("dir", "directory to gather schemas from (must be .json files)").ExistingDirs()
	files   = kingpin.Flag("file", "file to gather schemas from (must be a .json file)").ExistingFiles()
)

func main() {
	_ = kingpin.Parse()

	builder := vjsonschema.NewBuilder()
	for _, d := range *dirs {
		if err := builder.AddDir(d); err != nil {
			panic(errors.WithMessage(err, "failed to add directory"))
		}
	}
	for _, f := range *files {
		if err := builder.AddFile(f); err != nil {
			panic(errors.WithMessage(err, "failed to add file"))
		}
	}
	b, err := vjsmodels.Generate(*pkg, builder.GetSchemas())
	if err != nil {
		panic(errors.WithMessage(err, "failed to generate models"))
	}
	if err = ioutil.WriteFile(*outfile, b, 0744); err != nil {
		panic(errors.WithMessage(err, "failed to write models to file"))
	}
}
