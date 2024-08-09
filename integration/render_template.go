package main

import (
	"fmt"
	"os"
	"text/template"
)

type yamlInputs struct {
	BuildImage      string
	AdmiralLabel    string
	SecretFilterTag string
}

func main() {

	yaml := yamlInputs{
		BuildImage:      os.Getenv("ADMIRAL_BUILD_IMAGE"),
		AdmiralLabel:    os.Getenv("ADMIRAL_LABEL"),
		SecretFilterTag: os.Getenv("SECRET_FILTER_TAG"),
	}

	fmt.Println("Rendering template with the following inputs:")
	fmt.Println("BuildImage: ", yaml.BuildImage)
	fmt.Println("AdmiralLabel: ", yaml.AdmiralLabel)
	fmt.Println("SecretFilterTag: ", yaml.SecretFilterTag)

	// Create the file
	f, err := os.OpenFile("admiral_rendered_deployment.yaml", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Render the template
	var tmplFile = "admiral_deployment.tmpl"
	tmpl, err := template.New(tmplFile).ParseFiles(tmplFile)
	if err != nil {
		panic(err)
	}

	// Execute the template to the file
	err = tmpl.Execute(f, yaml)
	if err != nil {
		panic(err)
	}
}
