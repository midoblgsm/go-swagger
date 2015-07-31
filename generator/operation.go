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
	operationTemplate *template.Template
	parameterTemplate *template.Template
)

func init() {
	bv, _ := Asset("templates/server/parameter.gotmpl")
	parameterTemplate = template.Must(template.New("parameter").Parse(string(bv)))

	bm, _ := Asset("templates/server/operation.gotmpl")
	operationTemplate = template.Must(template.New("operation").Parse(string(bm)))
}

// GenerateServerOperation generates a parameter model, parameter validator, http handler implementations for a given operation
// It also generates an operation handler interface that uses the parameter model for handling a valid request.
// Allows for specifying a list of tags to include only certain tags for the generation
func GenerateServerOperation(operationNames, tags []string, includeHandler, includeParameters bool, opts GenOpts) error {
	// Load the spec
	specPath, specDoc, err := loadSpec(opts.Spec)
	if err != nil {
		return err
	}

	if len(operationNames) == 0 {
		operationNames = specDoc.OperationIDs()
	}

	for _, operationName := range operationNames {
		operation, ok := specDoc.OperationForName(operationName)
		if !ok {
			return fmt.Errorf("operation %q not found in %s", operationName, specPath)
		}

		generator := operationGenerator{
			Name:                 operationName,
			APIPackage:           opts.APIPackage,
			ModelsPackage:        opts.ModelPackage,
			ClientPackage:        opts.ClientPackage,
			ServerPackage:        opts.ServerPackage,
			TestPackage:          opts.TestPackage,
			Operation:            *operation,
			SecurityRequirements: specDoc.SecurityRequirementsFor(operation),
			Principal:            opts.Principal,
			Target:               filepath.Join(opts.Target, opts.APIPackage),
			Tags:                 tags,
			IncludeHandler:       includeHandler,
			IncludeParameters:    includeParameters,
			DumpData:             opts.DumpData,
		}
		if err := generator.Generate(); err != nil {
			return err
		}
	}
	return nil
}

// GenerateTestOperation generates test suits for operations
func GenerateTestOperation(operationNames, tags []string, includeHandler, includeParameters bool, opts GenOpts) error {
	// Load the spec
	specPath, specDoc, err := loadSpec(opts.Spec)
	if err != nil {
		return err
	}

	if len(operationNames) == 0 {
		operationNames = specDoc.OperationIDs()
	}

	for _, operationName := range operationNames {
		operation, ok := specDoc.OperationForName(operationName)
		if !ok {
			return fmt.Errorf("operation %q not found in %s", operationName, specPath)
		}

		generator := operationGenerator{
			Name:                 operationName,
			APIPackage:           opts.APIPackage,
			ModelsPackage:        opts.ModelPackage,
			ClientPackage:        opts.ClientPackage,
			ServerPackage:        opts.ServerPackage,
			TestPackage:          opts.TestPackage,
			Operation:            *operation,
			SecurityRequirements: specDoc.SecurityRequirementsFor(operation),
			Principal:            opts.Principal,
			Target:               filepath.Join(opts.Target, opts.APIPackage),
			Tags:                 tags,
			IncludeHandler:       includeHandler,
			IncludeParameters:    includeParameters,
			DumpData:             opts.DumpData,
		}
		if err := generator.Generate(); err != nil {
			return err
		}
	}
	return nil
}

type operationGenerator struct {
	Name                 string
	Authorized           bool
	APIPackage           string
	ModelsPackage        string
	ServerPackage        string
	ClientPackage        string
	TestPackage			 string
	Operation            spec.Operation
	SecurityRequirements []spec.SecurityRequirement
	Principal            string
	Target               string
	Tags                 []string
	data                 interface{}
	pkg                  string
	cname                string
	IncludeHandler       bool
	IncludeParameters    bool
	DumpData             bool
}

func (o *operationGenerator) Generate() error {
	// Build a list of codegen operations based on the tags,
	// the tag decides the actual package for an operation
	// the user specified package serves as root for generating the directory structure
	var operations []genOperation
	authed := len(o.SecurityRequirements) > 0
	for _, tag := range o.Operation.Tags {
		if len(o.Tags) == 0 {
			operations = append(operations, makeCodegenOperation(o.Name, tag, o.ModelsPackage, o.Principal, o.Target, o.Operation, authed))
			continue
		}
		for _, ft := range o.Tags {
			if ft == tag {
				operations = append(operations, makeCodegenOperation(o.Name, tag, o.ModelsPackage, o.Principal, o.Target, o.Operation, authed))
				break
			}
		}

	}
	if len(operations) == 0 {
		operations = append(operations, makeCodegenOperation(o.Name, o.APIPackage, o.ModelsPackage, o.Principal, o.Target, o.Operation, authed))
	}

	for _, op := range operations {
		if o.DumpData {
			bb, _ := json.MarshalIndent(swag.ToDynamicJSON(op), "", " ")
			fmt.Fprintln(os.Stdout, string(bb))
			continue
		}
		o.data = op
		o.pkg = op.Package
		o.cname = op.ClassName

		if o.IncludeHandler {
			if err := o.generateHandler(); err != nil {
				return fmt.Errorf("handler: %s", err)
			}
			log.Println("generated handler", op.Package+"."+op.ClassName)
		}

		if o.IncludeParameters && len(o.Operation.Parameters) > 0 {
			if err := o.generateParameterModel(); err != nil {
				return fmt.Errorf("parameters: %s", err)
			}
			log.Println("generated parameters", op.Package+"."+op.ClassName+"Parameters")
		}

		if len(o.Operation.Parameters) == 0 {
			log.Println("no parameters for operation", op.Package+"."+op.ClassName)
		}
	}

	return nil
}

func (o *operationGenerator) generateHandler() error {
	buf := bytes.NewBuffer(nil)

	if err := operationTemplate.Execute(buf, o.data); err != nil {
		return err
	}
	log.Println("rendered handler template:", o.pkg+"."+o.cname)

	fp := filepath.Join(o.ServerPackage, o.Target)
	if len(o.Operation.Tags) > 0 {
		fp = filepath.Join(fp, o.pkg)
	}
	return writeToFile(fp, o.Name, buf.Bytes())
}

func (o *operationGenerator) generateParameterModel() error {
	buf := bytes.NewBuffer(nil)

	if err := parameterTemplate.Execute(buf, o.data); err != nil {
		return err
	}
	log.Println("rendered parameters template:", o.pkg+"."+o.cname+"Parameters")

	fp := filepath.Join(o.ServerPackage, o.Target)
	if len(o.Operation.Tags) > 0 {
		fp = filepath.Join(fp, o.pkg)
	}
	return writeToFile(fp, o.Name+"Parameters", buf.Bytes())
}

func makeCodegenOperation(name, pkg, modelsPkg, principal, target string, operation spec.Operation, authorized bool) genOperation {
	receiver := "o"

	var params, qp, pp, hp, fp []genParameter
	var hasQueryParams bool
	for _, p := range operation.Parameters {
		cp := makeCodegenParameter(receiver, modelsPkg, p)
		if cp.IsQueryParam {
			hasQueryParams = true
			qp = append(qp, cp)
		}
		if cp.IsFormParam {
			fp = append(fp, cp)
		}
		if cp.IsPathParam {
			pp = append(pp, cp)
		}
		if cp.IsHeaderParam {
			hp = append(hp, cp)
		}
		params = append(params, cp)
	}

	var successModel string
	var returnsPrimitive, returnsFormatted, returnsContainer, returnsMap bool
	if operation.Responses != nil {
		if r, ok := operation.Responses.StatusCodeResponses[200]; ok {
			tn := typeForSchema(r.Schema, modelsPkg)
			_, returnsPrimitive = primitives[tn]
			_, returnsFormatted = customFormatters[tn]
			returnsContainer = r.Schema.Items != nil || r.Schema.Type.Contains("array")
			returnsMap = strings.HasPrefix(tn, "map")
			successModel = tn
		}
	}

	prin := principal
	if prin == "" {
		prin = "interface{}"
	}

	zero, ok := zeroes[successModel]
	if !ok {
		zero = "nil"
	}

	return genOperation{
		Package:        pkg,
		ClassName:      swag.ToGoName(name),
		Name:           swag.ToJSONName(name),
		Description:    operation.Description,
		DocString:      operationDocString(swag.ToGoName(name), operation),
		ReceiverName:   receiver,
		HumanClassName: swag.ToHumanNameLower(swag.ToGoName(name)),
		DefaultImports: []string{
			filepath.ToSlash(filepath.Join(baseImport(filepath.Join(target, "..")), modelsPkg)),
			"github.com/go-swagger/go-swagger/httpkit/middleware",
			"github.com/go-swagger/go-swagger/strfmt",
		},
		Params:               params,
		Summary:              operation.Summary,
		QueryParams:          qp,
		PathParams:           pp,
		HeaderParams:         hp,
		FormParams:           fp,
		HasQueryParams:       hasQueryParams,
		SuccessModel:         successModel,
		SuccessZero:          zero,
		ReturnsPrimitive:     returnsPrimitive,
		ReturnsFormatted:     returnsFormatted,
		ReturnsContainer:     returnsContainer,
		ReturnsMap:           returnsMap,
		ReturnsComplexObject: !returnsPrimitive && !returnsFormatted && !returnsContainer && !returnsMap,
		Authorized:           authorized,
		Principal:            prin,
	}
}

func operationDocString(name string, operation spec.Operation) string {
	hdr := fmt.Sprintf("%s %s", name, operation.Description)
	ed := operation.ExternalDocs
	var txtFoot string
	if ed != nil {
		if ed.Description != "" && ed.URL != "" {
			txtFoot = fmt.Sprintf("\n%s\nSee: %s", ed.Description, ed.URL)
		}
		if ed.URL != "" {
			txtFoot = "\nSee: " + ed.URL
		}
	}
	return commentedLines(strings.Join([]string{hdr, txtFoot}, "\n"))
}

type genOperation struct {
	Package        string //`json:"package,omitempty"`        // -
	ReceiverName   string //`json:"receiverName,omitempty"`   // -
	ClassName      string //`json:"classname,omitempty"`      // -
	Name           string //`json:"name,omitempty"`           // -
	HumanClassName string //`json:"humanClassname,omitempty"` // -

	Summary      string //`json:"summary,omitempty"`
	Description  string //`json:"description,omitempty"` // -
	DocString    string //`json:"docString,omitempty"`   // -
	ExternalDocs string //`json:"externalDocs,omitempty"`

	Imports        map[string]string //`json:"imports,omitempty"` // -
	DefaultImports []string          //`json:"defaultImports,omitempty"` // -

	Authorized bool   //`json:"authorized"`          // -
	Principal  string //`json:"principal,omitempty"` // -

	SuccessModel         string //`json:"successModel,omitempty"`         // -
	SuccessZero          string //`json:"successZero,omitempty"`         // -
	ReturnsPrimitive     bool   //`json:"returnsPrimitive,omitempty"`     // -
	ReturnsFormatted     bool   //`json:"returnsFormatted,omitempty"`     // -
	ReturnsContainer     bool   //`json:"returnsContainer,omitempty"`     // -
	ReturnsComplexObject bool   //`json:"returnsComplexObject,omitempty"` // -
	ReturnsMap           bool   //`json:"returnsMap,omitempty"`

	Params         []genParameter //`json:"params,omitempty"`         // -
	QueryParams    []genParameter //`json:"queryParams,omitempty"`    // -
	PathParams     []genParameter //`json:"pathParams,omitempty"`     // -
	HeaderParams   []genParameter //`json:"headerParams,omitempty"`   // -
	FormParams     []genParameter //`json:"formParams,omitempty"`     // -
	HasQueryParams bool           //`json:"hasQueryParams,omitempty"` // -
	HasFormParams  bool           //`json:"hasFormParams,omitempty"`  // -
	HasFileParams  bool           //`json:"hasFileParams,omitempty"`  // -
}

func makeCodegenParameter(receiver, modelsPkg string, param spec.Parameter) genParameter {
	var ctx sharedParam
	var child *genParameterItem

	if param.In == "body" {
		ctx = makeGenValidations(modelValidations(
			"\""+swag.ToJSONName(param.Name)+"\"",
			swag.ToJSONName(param.Name),
			swag.ToGoName(param.Name),
			"i",
			receiver+"."+swag.ToGoName(param.Name),
			modelsPkg,
			param.Required,
			*param.Schema))

	} else {
		ctx = makeGenValidations(paramValidations(receiver, param))
		thisItem := genParameterItem{}
		thisItem.sharedParam = ctx
		thisItem.ValueExpression = ctx.IndexVar + "c"
		thisItem.CollectionFormat = param.CollectionFormat
		thisItem.Converter = stringConverters[ctx.Type]
		thisItem.Location = param.In

		if param.Items != nil {
			it := makeCodegenParamItem(
				"fmt.Sprintf(\"%s.%v\", "+ctx.Path+", "+ctx.IndexVar+")",
				ctx.ParamName,
				ctx.PropertyName,
				ctx.IndexVar+"i",
				ctx.IndexVar+"c["+ctx.IndexVar+"]",
				thisItem,
				*param.Items,
			)
			child = &it
		}

	}

	return genParameter{
		sharedParam:      ctx,
		Description:      param.Description,
		ReceiverName:     receiver,
		IsQueryParam:     param.In == "query",
		IsBodyParam:      param.In == "body",
		IsHeaderParam:    param.In == "header",
		IsPathParam:      param.In == "path",
		IsFormParam:      param.In == "formData",
		IsFileParam:      param.Type == "file",
		CollectionFormat: param.CollectionFormat,
		Child:            child,
		Location:         param.In,
		Converter:        stringConverters[ctx.Type],
	}
}

type genParameter struct {
	sharedParam
	ReceiverName     string            //`json:"receiverName,omitempty"`
	Description      string            //`json:"description,omitempty"`
	IsQueryParam     bool              //`json:"isQueryParam,omitempty"`
	IsFormParam      bool              // `json:"isFormParam,omitempty"`
	IsPathParam      bool              //`json:"isPathParam,omitempty"`
	IsHeaderParam    bool              //`json:"isHeaderParam,omitempty"`
	IsBodyParam      bool              //`json:"isBodyParam,omitempty"`
	IsFileParam      bool              //`json:"isFileParam,omitempty"`
	CollectionFormat string            //`json:"collectionFormat,omitempty"`
	Child            *genParameterItem //`json:"child,omitempty"`
	BodyParam        *genParameter     //`json:"bodyParam,omitempty"`
	Converter        string            //`json:"converter,omitempty"`
	Parent           *genParameterItem //`json:"parent,omitempty"` // this is meant to be nil, just here for completeness in the templates
	Location         string            //`json:"location,omitempty"`
}

func makeCodegenParamItem(path, paramName, accessor, indexVar, valueExpression string, parent genParameterItem, items spec.Items) genParameterItem {
	ctx := makeGenValidations(paramItemValidations(path, paramName, accessor, indexVar, valueExpression, items))

	res := genParameterItem{}
	res.sharedParam = ctx
	res.CollectionFormat = items.CollectionFormat
	res.Parent = &parent
	res.Converter = stringConverters[ctx.Type]
	res.Location = parent.Location
	res.ValueExpression = "value"

	var child *genParameterItem
	if items.Items != nil {
		it := makeCodegenParamItem(
			"fmt.Sprintf(\"%s.%v\", "+ctx.Path+", "+ctx.IndexVar+")",
			ctx.ParamName,
			ctx.PropertyName,
			ctx.IndexVar+"i",
			ctx.IndexVar+"c["+ctx.IndexVar+"]",
			res,
			*items.Items,
		)
		child = &it
	}
	res.Child = child

	return res
}

type genParameterItem struct {
	sharedParam
	CollectionFormat string            //`json:"collectionFormat,omitempty"`
	Child            *genParameterItem //`json:"child,omitempty"`
	Parent           *genParameterItem //`json:"parent,omitempty"`
	Converter        string            //`json:"converter,omitempty"`
	Location         string            //`json:"location,omitempty"`
}

type sharedParam struct {
	genValidations
	propertyDescriptor
}

func paramItemValidations(path, paramName, accessor, indexVar, valueExpression string, items spec.Items) commonValidations {
	tpe := resolveSimpleType(items.Type, items.Format, items.Items)
	_, isPrimitive := primitives[tpe]
	_, isCustomFormatter := customFormatters[tpe]

	return commonValidations{
		propertyDescriptor: propertyDescriptor{
			PropertyName:      accessor,
			ParamName:         paramName,
			ValueExpression:   valueExpression,
			IndexVar:          indexVar,
			Path:              path,
			IsContainer:       items.Items != nil || tpe == "array",
			IsPrimitive:       isPrimitive,
			IsCustomFormatter: isCustomFormatter,
			IsMap:             strings.HasPrefix(tpe, "map"),
		},

		Type:             tpe,
		Format:           items.Format,
		Items:            items.Items,
		Default:          items.Default,
		Maximum:          items.Maximum,
		ExclusiveMaximum: items.ExclusiveMaximum,
		Minimum:          items.Minimum,
		ExclusiveMinimum: items.ExclusiveMinimum,
		MaxLength:        items.MaxLength,
		MinLength:        items.MinLength,
		Pattern:          items.Pattern,
		MaxItems:         items.MaxItems,
		MinItems:         items.MinItems,
		UniqueItems:      items.UniqueItems,
		MultipleOf:       items.MultipleOf,
		Enum:             items.Enum,
	}
}

func paramValidations(receiver string, param spec.Parameter) commonValidations {
	accessor := swag.ToGoName(param.Name)
	paramName := swag.ToJSONName(param.Name)

	tpe := typeForParameter(param)
	_, isPrimitive := primitives[tpe]
	_, isCustomFormatter := customFormatters[tpe]

	return commonValidations{
		propertyDescriptor: propertyDescriptor{
			PropertyName:      accessor,
			ParamName:         paramName,
			ValueExpression:   fmt.Sprintf("%s.%s", receiver, accessor),
			IndexVar:          "i",
			Path:              "\"" + paramName + "\"",
			IsContainer:       param.Items != nil || tpe == "array",
			IsPrimitive:       isPrimitive,
			IsCustomFormatter: isCustomFormatter,
			IsMap:             strings.HasPrefix(tpe, "map"),
		},
		Required:         param.Required,
		Type:             tpe,
		Format:           param.Format,
		Items:            param.Items,
		Default:          param.Default,
		Maximum:          param.Maximum,
		ExclusiveMaximum: param.ExclusiveMaximum,
		Minimum:          param.Minimum,
		ExclusiveMinimum: param.ExclusiveMinimum,
		MaxLength:        param.MaxLength,
		MinLength:        param.MinLength,
		Pattern:          param.Pattern,
		MaxItems:         param.MaxItems,
		MinItems:         param.MinItems,
		UniqueItems:      param.UniqueItems,
		MultipleOf:       param.MultipleOf,
		Enum:             param.Enum,
	}
}

func makeGenValidations(s commonValidations) sharedParam {
	hasValidations := s.Required

	var defVal string
	if s.Default != nil {
		hasValidations = false
		defVal = fmt.Sprintf("%#v", s.Default)
	}

	var format string
	if s.IsCustomFormatter {
		hasValidations = true
		format = s.Format
	}

	var maxLength int64
	if s.MaxLength != nil {
		hasValidations = true
		maxLength = *s.MaxLength
	}

	var minLength int64
	if s.MinLength != nil {
		hasValidations = true
		minLength = *s.MinLength
	}

	var minimum float64
	if s.Minimum != nil {
		hasValidations = true
		minimum = *s.Minimum
	}

	var maximum float64
	if s.Maximum != nil {
		hasValidations = true
		maximum = *s.Maximum
	}

	var multipleOf float64
	if s.MultipleOf != nil {
		hasValidations = true
		multipleOf = *s.MultipleOf
	}

	var needsSize bool
	hasSliceValidations := s.UniqueItems
	var maxItems int64
	if s.MaxItems != nil {
		hasSliceValidations = true
		needsSize = true
		maxItems = *s.MaxItems
	}

	var minItems int64
	if s.MinItems != nil {
		hasSliceValidations = true
		needsSize = true
		minItems = *s.MinItems
	}

	var enum string
	if len(s.Enum) > 0 {
		hasValidations = true
		enum = fmt.Sprintf("%#v", s.Enum)
	}

	return sharedParam{
		propertyDescriptor: s.propertyDescriptor,
		genValidations: genValidations{
			Type:                s.Type,
			Required:            s.Required,
			DefaultValue:        defVal,
			MaxLength:           maxLength,
			MinLength:           minLength,
			Pattern:             s.Pattern,
			MultipleOf:          multipleOf,
			Minimum:             minimum,
			Maximum:             maximum,
			ExclusiveMinimum:    s.ExclusiveMinimum,
			ExclusiveMaximum:    s.ExclusiveMaximum,
			Enum:                enum,
			HasValidations:      hasValidations,
			Format:              format,
			MinItems:            minItems,
			MaxItems:            maxItems,
			UniqueItems:         s.UniqueItems,
			HasSliceValidations: hasSliceValidations,
			NeedsSize:           needsSize,
		},
	}
}
