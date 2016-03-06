package filters

import (
	"bytes"
	log "github.com/Sirupsen/logrus"
	"text/template"
)

type ParamTemplate struct {
	*template.Template
}

func NewParamTemplate(txt string) (*ParamTemplate, error) {
	t := template.New(txt) // use self as name
	t, err := t.Parse(txt)
	if err != nil {
		return nil, err
	}

	return &ParamTemplate{t}, nil
}

func (pt *ParamTemplate) Execute(params map[string]string) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := pt.Template.Execute(buf, params)
	return buf.Bytes(), err
}

func (pt *ParamTemplate) ExecuteLogged(params map[string]string) ([]byte, bool) {
	b, err := pt.Execute(params)
	if err != nil {
		log.Error(err)
	}

	return b, err == nil
}
