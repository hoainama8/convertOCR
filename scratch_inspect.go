package main

import (
	"fmt"
	"reflect"

	docxgo "github.com/mmonterroca/docxgo/v2"
)

func main() {
	doc := docxgo.NewDocument()
	t := reflect.TypeOf(doc)
	fmt.Printf("Methods for %v:\n", t)
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		fmt.Printf(" - %s\n", m.Name)
	}
}
