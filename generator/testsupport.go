package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-swagger/go-swagger/spec"
	"github.com/go-swagger/go-swagger/swag"
)

var (
	suiteTestTemplate      *template.Template
	testTemplate         *template.Template
	tckTemplate         *template.Template
)

func init() {

	ts, _ := Asset("templates/test/tmp_suite_test.gotmpl")
	suiteTestTemplate = template.Must(template.New("tmp_suite_test").Parse(string(ts)))

	t, _ := Asset("templates/test/tmp_test.gotmpl")
	testTemplate = template.Must(template.New("tmp_test").Parse(string(t)))

	tckr, _ := Asset("templates/tck/tckReporter.gotmpl")
	tckTemplate = template.Must(template.New("tmp_tck").Parse(string(tckr)))
}

// GenerateSupport generates the supporting files for an API
func GenerateTestSupport(name string, modelNames, operationIDs []string, includeUI bool,includeTCK bool, opts GenOpts) error {
		// Load the spec
	_, specDoc, err := loadSpec(opts.Spec)
	if err != nil {
		return err
	}

	models, mnc := make(map[string]spec.Schema), len(modelNames)
	for k, v := range specDoc.Spec().Definitions {
		for _, nm := range modelNames {
			if mnc == 0 || k == nm {
				models[k] = v
			}
		}
	}

	operations := make(map[string]spec.Operation)
	if len(modelNames) == 0 {
		for _, k := range specDoc.OperationIDs() {
			if op, ok := specDoc.OperationForName(k); ok {
				operations[k] = *op
			}
		}
	} else {
		for _, k := range specDoc.OperationIDs() {
			for _, nm := range operationIDs {
				if k == nm {
					if op, ok := specDoc.OperationForName(k); ok {
						operations[k] = *op
					}
				}
			}
		}
	}

	if name == "" {
		if specDoc.Spec().Info != nil && specDoc.Spec().Info.Title != "" {
			name = swag.ToGoName(specDoc.Spec().Info.Title)
		} else {
			name = "swagger"
		}
	}

	generator := testGenerator{
		Name:       name,
		SpecDoc:    specDoc,
		Models:     models,
		Operations: operations,
		Target:     opts.Target,
		// Package:       filepath.Base(opts.Target),
		DumpData:      opts.DumpData,
		Package:       opts.APIPackage,
		APIPackage:    opts.APIPackage,
		ModelsPackage: opts.ModelPackage,
		ServerPackage: opts.ServerPackage,
		ClientPackage: opts.ClientPackage,
		Principal:     opts.Principal,
		IncludeUI:     includeUI,
		IncludeTCK:    includeTCK,
	}

	return generator.GenerateTest()
}

type testGenerator struct {
	Name          string
	SpecDoc       *spec.Document
	Package       string
	APIPackage    string
	ModelsPackage string
	ServerPackage string
	ClientPackage string
	Principal     string
	Models        map[string]spec.Schema
	Operations    map[string]spec.Operation
	Target        string
	DumpData      bool
	IncludeUI     bool
	IncludeTCK	  bool
}


func (t *testGenerator) GenerateTest() error {
	test := t.makeCodegenTest()

	if t.DumpData {
		bb, _ := json.MarshalIndent(swag.ToDynamicJSON(test), "", "  ")
		fmt.Fprintln(os.Stdout, string(bb))
		return nil
	}

	test.DefaultImports = append(test.DefaultImports, filepath.Join(baseImport(t.Target), t.ServerPackage, t.APIPackage))


	if t.IncludeTCK {
	if err := t.generateTCK(&test); err != nil {
		return err
	}
}
	if err := t.generateSuiteTest(&test); err != nil {
		return err
	}
	
	for _, operation := range test.Operations {
		test.OperationID = operation.Name
		if err := t.generateTest(&test); err != nil {
			return err
		}		
	}


	return nil
}

func (t *testGenerator) generateSuiteTest(test *genTest) error {
	pth := filepath.Join(t.Target, "cmd", swag.ToCommandName(test.AppName))
	nm := test.AppName + "_suite_test"
	buf := bytes.NewBuffer(nil)
	if err := suiteTestTemplate.Execute(buf, test); err != nil {
		return err
	}
	log.Println("rendered suite test template:", test.Package + test.AppName + "_suite_test")
	return writeToFileIfNotExist(pth, nm, buf.Bytes())
}

func (t *testGenerator) generateTCK(test *genTest) error {
	pth := filepath.Join(t.Target, "cmd", swag.ToCommandName(test.AppName))
	nm :=  "tck_reporter"
	buf := bytes.NewBuffer(nil)
	if err := tckTemplate.Execute(buf, test); err != nil {
		return err
	}
	log.Println("rendered tck template:", test.Package + "tck_reporter")
	return writeToFileIfNotExist(pth, nm, buf.Bytes())
}

func (t *testGenerator) generateTest(test *genTest) error {
	buf := bytes.NewBuffer(nil)
	if err := testTemplate.Execute(buf, test); err != nil {
		return err
	}
	log.Println("rendered test template:", "test." + test.OperationID)
	return writeToFile(filepath.Join(t.Target, "cmd", swag.ToCommandName(test.AppName)), test.OperationID + "_test", buf.Bytes())
}

func (t *testGenerator) makeCodegenTest() genTest {
	sw := t.SpecDoc.Spec()
	// app := makeCodegenApp(t.Operations, t.IncludeUI)
	receiver := strings.ToLower(t.Name[:1])
	appName := swag.ToGoName(t.Name)
	var defaultImports []string

	jsonb, _ := json.MarshalIndent(t.SpecDoc.Spec(), "", "  ")

	consumesJSON := false
	var consumes []genSerGroup
	for _, cons := range t.SpecDoc.RequiredConsumes() {
		cn, ok := mediaTypeNames[cons]
		if !ok {
			continue
		}
		nm := swag.ToJSONName(cn)
		if nm == "json" {
			consumesJSON = true
		}

		if ser, ok := getSerializer(consumes, cn); ok {
			ser.AllSerializers = append(ser.AllSerializers, genSerializer{
				AppName:        ser.AppName,
				ReceiverName:   ser.ReceiverName,
				ClassName:      ser.ClassName,
				HumanClassName: ser.HumanClassName,
				Name:           ser.Name,
				MediaType:      cons,
				Implementation: knownConsumers[nm],
			})
			continue
		}

		ser := genSerializer{
			AppName:        appName,
			ReceiverName:   receiver,
			ClassName:      swag.ToGoName(cn),
			HumanClassName: swag.ToHumanNameLower(cn),
			Name:           nm,
			MediaType:      cons,
			Implementation: knownConsumers[nm],
		}

		consumes = append(consumes, genSerGroup{
			AppName:        ser.AppName,
			ReceiverName:   ser.ReceiverName,
			ClassName:      ser.ClassName,
			HumanClassName: ser.HumanClassName,
			Name:           ser.Name,
			MediaType:      cons,
			AllSerializers: []genSerializer{ser},
			Implementation: ser.Implementation,
		})
	}

	producesJSON := false
	var produces []genSerGroup
	for _, prod := range t.SpecDoc.RequiredProduces() {
		pn, ok := mediaTypeNames[prod]
		if !ok {
			continue
		}
		nm := swag.ToJSONName(pn)
		if nm == "json" {
			producesJSON = true
		}

		if ser, ok := getSerializer(produces, pn); ok {
			ser.AllSerializers = append(ser.AllSerializers, genSerializer{
				AppName:        ser.AppName,
				ReceiverName:   ser.ReceiverName,
				ClassName:      ser.ClassName,
				HumanClassName: ser.HumanClassName,
				Name:           ser.Name,
				MediaType:      prod,
				Implementation: knownProducers[nm],
			})
			continue
		}
		ser := genSerializer{
			AppName:        appName,
			ReceiverName:   receiver,
			ClassName:      swag.ToGoName(pn),
			HumanClassName: swag.ToHumanNameLower(pn),
			Name:           nm,
			MediaType:      prod,
			Implementation: knownProducers[nm],
		}
		produces = append(produces, genSerGroup{
			AppName:        ser.AppName,
			ReceiverName:   ser.ReceiverName,
			ClassName:      ser.ClassName,
			HumanClassName: ser.HumanClassName,
			Name:           ser.Name,
			MediaType:      prod,
			Implementation: ser.Implementation,
			AllSerializers: []genSerializer{ser},
		})
	}

	var security []genSecurityScheme
	for _, scheme := range t.SpecDoc.RequiredSchemes() {
		if req, ok := t.SpecDoc.Spec().SecurityDefinitions[scheme]; ok {
			if req.Type == "basic" || req.Type == "apiKey" {
				security = append(security, genSecurityScheme{
					AppName:        appName,
					ReceiverName:   receiver,
					ClassName:      swag.ToGoName(req.Name),
					HumanClassName: swag.ToHumanNameLower(req.Name),
					Name:           swag.ToJSONName(req.Name),
					IsBasicAuth:    strings.ToLower(req.Type) == "basic",
					IsAPIKeyAuth:   strings.ToLower(req.Type) == "apikey",
					Principal:      t.Principal,
					Source:         req.In,
				})
			}
		}
	}

	var genMods []genModel
	defaultImports = append(defaultImports, filepath.Join(baseImport(t.Target), t.ModelsPackage))
	for mn, m := range t.Models {
		mod := *makeCodegenModel(
			mn,
			t.ModelsPackage,
			m,
			t.SpecDoc,
		)
		mod.ReceiverName = receiver
		genMods = append(genMods, mod)
	}

	var genOps []genOperation
	tns := make(map[string]struct{})
	for on, o := range t.Operations {
		//fmt.Printf("====> %+v\n", o)

		authed := len(t.SpecDoc.SecurityRequirementsFor(&o)) > 0
		ap := t.APIPackage
		if t.APIPackage == t.Package {
			ap = ""
		}
		if len(o.Tags) > 0 {
			for _, tag := range o.Tags {
				tns[tag] = struct{}{}
				op := makeCodegenOperation(on, tag, t.ModelsPackage, t.Principal, t.Target, o, authed)
				op.ReceiverName = receiver
				genOps = append(genOps, op)
			}
		} else {
			op := makeCodegenOperation(on, ap, t.ModelsPackage, t.Principal, t.Target, o, authed)
			op.ReceiverName = receiver
			genOps = append(genOps, op)
		}
	}
	for k := range tns {
		defaultImports = append(defaultImports, filepath.Join(baseImport(t.Target), t.ServerPackage, t.APIPackage, k))
	}

	defaultConsumes := "application/json"
	rc := t.SpecDoc.RequiredConsumes()
	if !consumesJSON && len(rc) > 0 {
		defaultConsumes = rc[0]
	}

	defaultProduces := "application/json"
	rp := t.SpecDoc.RequiredProduces()
	if !producesJSON && len(rp) > 0 {
		defaultProduces = rp[0]
	}

	return genTest{
		Package:             t.Package,
		ReceiverName:        receiver,
		AppName:             swag.ToGoName(t.Name),
		HumanAppName:        swag.ToHumanNameLower(t.Name),
		Name:                swag.ToJSONName(t.Name),
		ExternalDocs:        sw.ExternalDocs,
		Info:                sw.Info,
		Consumes:            consumes,
		Produces:            produces,
		DefaultConsumes:     defaultConsumes,
		DefaultProduces:     defaultProduces,
		DefaultImports:      defaultImports,
		SecurityDefinitions: security,
		Models:              genMods,
		Operations:          genOps,
		IncludeUI:           t.IncludeUI,
		IncludeTCK:          t.IncludeTCK,
		Principal:           t.Principal,
		SwaggerJSON:         fmt.Sprintf("%#v", jsonb),
	}
}

type genTest struct {
	Package             string
	ReceiverName        string
	AppName             string
	HumanAppName        string
	Name                string
	Principal           string
	DefaultConsumes     string
	DefaultProduces     string
	Info                *spec.Info
	ExternalDocs        *spec.ExternalDocumentation
	Imports             map[string]string
	DefaultImports      []string
	Consumes            []genSerGroup
	Produces            []genSerGroup
	SecurityDefinitions []genSecurityScheme
	Models              []genModel
	Operations          []genOperation
	OperationID 		string
	IncludeUI           bool
	IncludeTCK          bool
	SwaggerJSON         string
}

