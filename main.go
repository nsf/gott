package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Masterminds/sprig/v3"
	"io"
	"os"
	"strconv"
	"strings"
	"text/template"
	"unicode/utf8"
)

type VarDefsFlag []string

func (i *VarDefsFlag) String() string {
	return "my string representation"
}

func (i *VarDefsFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var varDefs VarDefsFlag

func init() {
	flag.Var(&varDefs, "d", "Define variables, syntax: NAME[:TYPE[:TYPE]]=VALUE")
}

var inputFile = flag.String("f", "-", "Input file name or - for stdin")
var outputFile = flag.String("o", "-", "Output file name or - for stdout")
var versionFlag = flag.Bool("v", false, "Print the version")
var version = "v1.x.x"

func fatalf(format string, args ...interface{}) {
	err := fmt.Errorf(format, args...)
	os.Stderr.WriteString(err.Error() + "\n")
	os.Exit(1)
}

type TypeParser func(input string) (output interface{}, err error)

func int64TypeParser(input string) (output interface{}, err error) {
	i, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed parsing int64: %w", err)
	}
	return i, nil
}

func float64TypeParser(input string) (output interface{}, err error) {
	f, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return nil, fmt.Errorf("failed parsing float64: %w", err)
	}
	return f, nil
}

var typeParsers = map[string]TypeParser{
	"json": func(input string) (output interface{}, err error) {
		if err := json.Unmarshal([]byte(input), &output); err != nil {
			return nil, fmt.Errorf("failed parsing json: %w", err)
		}
		return
	},
	"string": func(input string) (output interface{}, err error) {
		return input, nil
	},
	"int":     int64TypeParser,
	"int64":   int64TypeParser,
	"float":   float64TypeParser,
	"float64": float64TypeParser,
	"bool": func(input string) (output interface{}, err error) {
		b, err := strconv.ParseBool(input)
		if err != nil {
			return nil, fmt.Errorf("failed parsing bool: %w", err)
		}
		return b, nil
	},
	"file": func(input string) (output interface{}, err error) {
		data, err := os.ReadFile(input)
		if err != nil {
			return nil, fmt.Errorf("failed loading file: %w", err)
		}
		if !utf8.Valid(data) {
			fatalf("file %q contains invalid utf-8", input)
		}
		return string(data), nil
	},
	"env": func(input string) (output interface{}, err error) {
		return os.Getenv(input), nil
	},
}

func parseVariableDefinition(spec string) (string, interface{}, error) {
	nameTypesAndValue := strings.SplitN(spec, "=", 2)
	if len(nameTypesAndValue) != 2 {
		return "", nil, fmt.Errorf("variable definition format is: NAME[:TYPE[:TYPE]]=VALUE")
	}
	nameTypes := nameTypesAndValue[0]
	value := nameTypesAndValue[1]

	nameAndTypes := strings.Split(nameTypes, ":")
	name := nameAndTypes[0]
	types := nameAndTypes[1:]
	if len(types) == 0 {
		// default type is string
		return name, value, nil
	} else {
		var val interface{}
		var err error
		for i := 0; i < len(types); i++ {
			t := types[len(types)-1-i]
			tp, ok := typeParsers[t]
			if !ok {
				return "", nil, fmt.Errorf("unsupported type: %q", t)
			}
			if i != 0 {
				value, ok = val.(string)
				if !ok {
					return "", nil, fmt.Errorf("when chaining types, output of the preceding type must be a string, but it is %T", val)
				}
			}
			val, err = tp(value)
			if err != nil {
				return "", nil, err
			}
		}
		return name, val, nil
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nVariable types:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  bool    - boolean, uses Go's strconv.ParseBool\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  env     - string, read value from environment variable (chainable)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  file    - string, read value from utf-8 file (chainable)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  float   - float64, uses Go's strconv.ParseFloat\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  float64 - float64, uses Go's strconv.ParseFloat\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  int     - int64, uses Go's strconv.ParseInt\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  int64   - int64, uses Go's strconv.ParseInt\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  json    - any, uses Go's encoding/json.Unmarshal\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  string  - string\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Variable definition examples:\n")
		fmt.Fprintf(flag.CommandLine.Output(), `  -d 'name=John'                         - define "name" string variable with "John" as value`+"\n")
		fmt.Fprintf(flag.CommandLine.Output(), `  -d 'debug:bool=false'                  - define "debug" boolean variable with false as value`+"\n")
		fmt.Fprintf(flag.CommandLine.Output(), `  -d 'config:json:file=/etc/config.json" - read file "/etc/config.json", parse it as json, save the result to "config" variable`+"\n")
		fmt.Fprintf(flag.CommandLine.Output(), `  -d 'IsRelease:bool:env=IS_RELEASE"     - read environment variable "IS_RELEASE", parse it as bool, save the result to "IsRelease" variable`+"\n")
		fmt.Fprintf(flag.CommandLine.Output(), `  -d 'a=1' -d 'b=2'                      - define multiple variables`+"\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  It's easiest to read it from right to left: NAME:A:B=VALUE - VALUE is applied to type B, then to type A, then saved as NAME.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  Variables are available from the top level template context object, e.g. {{ if .IsRelease }}RELEASE{{ else }}DEBUG{{ end }}\n")
	}
	flag.Parse()

	if *versionFlag {
		fmt.Fprintf(flag.CommandLine.Output(), "%s\n", version)
		return
	}

	// load template file data
	var data []byte
	var err error
	if *inputFile == "-" {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			fatalf("error reading template from stdin: %w", err)
		}
	} else {
		data, err = os.ReadFile(*inputFile)
		if err != nil {
			fatalf("error reading template from file %q: %w", *inputFile, err)
		}
	}

	// parse template
	if !utf8.Valid(data) {
		fatalf("template file %q contains invalid utf-8", *inputFile)
	}
	tpl, err := template.New("main").Funcs(sprig.TxtFuncMap()).Parse(string(data))
	if err != nil {
		fatalf("error parsing template %q: %w", *inputFile, err)
	}

	// parse and apply variable definitions
	context := map[string]interface{}{}
	for _, vd := range varDefs {
		name, val, err := parseVariableDefinition(string(vd))
		if err != nil {
			fatalf("error parsing variable definition %q: %w", vd, err)
		}
		context[name] = val
	}

	var output io.Writer
	if *outputFile == "-" {
		output = os.Stdout
	} else {
		f, err := os.Create(*outputFile)
		if err != nil {
			fatalf("failed creating file %q: %w", *outputFile, err)
		}
		defer f.Close()
		output = f
	}
	if err := tpl.Execute(output, context); err != nil {
		fatalf("failed executing template: %w", err)
	}
}
