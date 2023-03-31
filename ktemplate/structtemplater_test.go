package ktemplate

import (
	"reflect"
	"testing"

	"github.com/flanksource/commons/logger"
)

type Test struct {
	Template   string `template:"true"`
	NoTemplate string
	Inner      Inner
	Labels     map[string]string `template:"true"`
}

type Inner struct {
	Template   string `template:"true"`
	NoTemplate string
}

type test struct {
	name string
	StructTemplater
	Input, Output *Test
	Vars          map[string]string
}

var tests = []test{
	{
		StructTemplater: StructTemplater{
			RequiredTag: "template",
			Values: map[string]interface{}{
				"msg": "world",
			},
		},
		Input: &Test{
			Template:   "hello {{.msg}}",
			NoTemplate: "hello {{.msg}}",
		},
		Output: &Test{
			Template:   "hello world",
			NoTemplate: "hello {{.msg}}",
		},
	},
	{
		StructTemplater: StructTemplater{
			DelimSets: []Delims{
				{Left: "{{", Right: "}}"},
				{Left: "$(", Right: ")"},
			},
			Values: map[string]interface{}{
				"msg": "world",
			},
			ValueFunctions: true,
		},
		Input: &Test{
			Template: "hello $(msg)",
		},
		Output: &Test{
			Template: "hello world",
		},
	},
	{
		StructTemplater: StructTemplater{
			RequiredTag: "template",
			DelimSets: []Delims{
				{Left: "{{", Right: "}}"},
				{Left: "$(", Right: ")"},
			},
			Values: map[string]interface{}{
				"name":  "James Bond",
				"color": "blue",
				"code":  "007",
			},
			ValueFunctions: true,
		},
		Input: &Test{
			Template: "Hello, $(name)!",
			Labels: map[string]string{
				"color": "light $(color)",
				"code":  "{{code}}",
			},
		},
		Output: &Test{
			Template: "Hello, James Bond!",
			Labels: map[string]string{
				"color": "light blue",
				"code":  "007",
			},
		},
	},
}

func TestMain(t *testing.T) {
	logger.StandardLogger().SetLogLevel(2)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			i := test.Input
			if err := test.StructTemplater.Walk(i); err != nil {
				t.Error(err)
			} else if !reflect.DeepEqual(i, test.Output) {
				t.Errorf("expected %+v\tgot %+v", test.Output, test.Input)
			}
		})
	}
}
