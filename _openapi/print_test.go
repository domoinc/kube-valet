package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"text/template"

	"github.com/go-openapi/jsonreference"
	"github.com/go-openapi/spec"
)

func createDefinitionObj() map[string]interface{} {

	def := GetOpenAPIDefinitions(func(path string) spec.Ref {
		return spec.Ref{Ref: jsonreference.MustCreateRef(path)}
	})

	munged := make(map[string]interface{})

	//make openapi spec compliant

	for k := range def {
		munged[k] = def[k].Schema
	}

	bytes, err := json.MarshalIndent(munged, "", "  ")
	if err != nil {
		panic(err)
	}

	jsonStr := string(bytes)

	//fix path references
	jsonStr = strings.Replace(jsonStr, "k8s.io/apimachinery/pkg/apis/meta/", "", -1)
	jsonStr = strings.Replace(jsonStr, "github.com/domoinc/kube-valet/pkg/apis/", "", -1)
	jsonStr = strings.Replace(jsonStr, "k8s.io/api/core/", "", -1)
	jsonStr = strings.Replace(jsonStr, "assignments/v1alpha1", "assignments.v1alpha1", -1)
	jsonStr = strings.Replace(jsonStr, "$ref\": \"", "$ref\": \"#definitions/", -1)

	var definitions map[string]interface{}

	json.Unmarshal([]byte(jsonStr), &definitions)

	return definitions

}

type ApiContext struct {
	Plural  string
	Group   string
	Version string
	Name    string
}

var contexts = []ApiContext{
	ApiContext{
		Plural:  "nodeassignmentgroups",
		Group:   "assignment",
		Version: "v1alpha1",
		Name:    "NodeAssignmentGroup",
	},
	ApiContext{
		Plural:  "podassignmentrules",
		Group:   "assignment",
		Version: "v1alpha1",
		Name:    "PodAssignmentRule",
	},
	ApiContext{
		Plural:  "clusterpodassignmentrules",
		Group:   "assignment",
		Version: "v1alpha1",
		Name:    "ClusterPodAssignmentRule",
	}}

var fns = template.FuncMap{
	"last": func(x int, a interface{}) bool {
		return x == len(contexts)-1

	},
}

func createPathObj() map[string]interface{} {

	dat, err := ioutil.ReadFile("./path_template.tmpl")
	if err != nil {
		panic(err)
	}

	tmpl := template.Must(template.New("paths").Funcs(fns).Parse(string(dat)))

	var pathBytes bytes.Buffer

	err = tmpl.Execute(&pathBytes, contexts)
	if err != nil {
		panic(err)
	}

	fmt.Printf("found bytes %s %d\n", string(pathBytes.Bytes()), pathBytes.Len())

	paths := make(map[string]interface{})

	json.Unmarshal(pathBytes.Bytes(), &paths)

	return paths
}

func TestWriteOpenAPISpec(t *testing.T) {

	openapi := make(map[string]interface{})

	openapi["swagger"] = "2.0"
	openapi["info"] = map[string]interface{}{
		"title":   "kube-valet",
		"version": "v1alpha1",
	}
	openapi["paths"] = createPathObj()
	openapi["definitions"] = createDefinitionObj()

	bytes, err := json.MarshalIndent(openapi, "", "  ")
	if err != nil {
		panic(err)
	}

	err = ioutil.WriteFile("./openapi.json", bytes, 0666)

}
