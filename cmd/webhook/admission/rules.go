package admission

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/eskip"
	"gopkg.in/yaml.v2"
)

type (
	AdmissionRules struct {
		Rules []AdmissionRule `json:"rules"`
	}
	AdmissionRule struct {
		Reject string `json:"reject"`
		When   string `json:"when"`
	}
)

type (
	RuleAdmitter struct {
		rules []*compiledRule
	}

	compiledRule struct {
		reject  string
		program cel.Program
	}
)

var _ admitter = &RuleAdmitter{}

func ParseRulesYaml(rulesYaml []byte) (*AdmissionRules, error) {
	var ar AdmissionRules
	if err := yaml.Unmarshal(rulesYaml, &ar); err != nil {
		return nil, fmt.Errorf("failed to parse rules: %w", err)
	}
	return &ar, nil
}

func NewRuleAdmitter(rules *AdmissionRules) (*RuleAdmitter, error) {
	compiledRules := make([]*compiledRule, 0, len(rules.Rules))
	for i, rule := range rules.Rules {
		cr, err := compileRule(&rule)
		if err != nil {
			return nil, fmt.Errorf("failed to compile rule %d: %w", i, err)
		}
		compiledRules = append(compiledRules, cr)
	}
	return &RuleAdmitter{rules: compiledRules}, nil
}

func NewRuleAdmitterFrom(rulesFile string) (*RuleAdmitter, error) {
	rulesYaml, err := os.ReadFile(rulesFile)
	if err != nil {
		return nil, err
	}

	rules, err := ParseRulesYaml(rulesYaml)
	if err != nil {
		return nil, err
	}

	return NewRuleAdmitter(rules)
}

func compileRule(rule *AdmissionRule) (*compiledRule, error) {
	env, err := cel.NewEnv(
		cel.Variable("object", cel.MapType(cel.StringType, cel.DynType)),
		//ext.Strings(),
		cel.Function("eskipFilters", cel.MemberOverload("string_eskipFilters",
			[]*cel.Type{cel.StringType}, cel.ListType(cel.DynType),
			cel.UnaryBinding(eskipFilters),
		)),
		cel.Function("eskipFilter", cel.MemberOverload("string_eskipFilter",
			[]*cel.Type{cel.StringType}, cel.DynType,
			cel.UnaryBinding(eskipFilter),
		)),
		cel.Function("eskipRoutes", cel.MemberOverload("string_eskipRoutes",
			[]*cel.Type{cel.StringType}, cel.ListType(cel.DynType),
			cel.UnaryBinding(eskipRoutes),
		)),
	)
	if err != nil {
		return nil, err
	}

	ast, issues := env.Compile(rule.When)
	if issues.Err() != nil {
		return nil, fmt.Errorf("expression compile error: %w", issues.Err())
	}

	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("wrong expression output type: %v", ast.OutputType())
	}

	program, err := env.Program(ast, cel.EvalOptions(cel.OptOptimize))
	if err != nil {
		return nil, fmt.Errorf("program construction error: %w", err)
	}

	return &compiledRule{
		reject:  rule.Reject,
		program: program,
	}, nil
}

func eskipFilters(value ref.Val) ref.Val {
	ff, err := eskip.ParseFilters(value.Value().(string))
	if err != nil {
		return types.WrapErr(fmt.Errorf("eskipFilters: %w", err))
	}

	var m []map[string]any
	if err := convert(ff, &m); err != nil {
		return types.WrapErr(fmt.Errorf("eskipFilters: %w", err))
	}
	return types.DefaultTypeAdapter.NativeToValue(m)
}

func eskipFilter(value ref.Val) ref.Val {
	ff, err := eskip.ParseFilters(value.Value().(string))
	if err != nil {
		return types.WrapErr(fmt.Errorf("eskipFilter: %w", err))
	}
	if len(ff) != 1 {
		return types.WrapErr(fmt.Errorf("eskipFilter: requires single filter"))
	}

	var m map[string]any
	if err := convert(ff[0], &m); err != nil {
		return types.WrapErr(fmt.Errorf("eskipFilter: %w", err))
	}
	return types.DefaultTypeAdapter.NativeToValue(m)
}

func eskipRoutes(value ref.Val) ref.Val {
	rr, err := eskip.Parse(value.Value().(string))
	if err != nil {
		return types.WrapErr(fmt.Errorf("eskipRoutes: %w", err))
	}

	var m []map[string]any
	if err := convert(rr, &m); err != nil {
		return types.WrapErr(fmt.Errorf("eskipRoutes: %w", err))
	}
	return types.DefaultTypeAdapter.NativeToValue(m)
}

func convert(value any, resultPtr any) error {
	if b, err := json.Marshal(value); err != nil {
		return err
	} else if err := json.Unmarshal(b, resultPtr); err != nil {
		return err
	}
	return nil
}

func (a *RuleAdmitter) name() string {
	return "rules"
}

func (a *RuleAdmitter) admit(req *admissionRequest) (*admissionResponse, error) {
	// convert req to map[string]any
	object := make(map[string]any)
	if err := json.Unmarshal(req.Object, &object); err != nil {
		return nil, fmt.Errorf("failed to convert request object: %w", err)
	}

	var rejectMessages []string
	for i, rule := range a.rules {
		matches, _, err := rule.program.Eval(map[string]any{"object": object})
		if err != nil {
			log.Errorf("Failed to evaluate rule %d: %v", i, err)
			return nil, errors.New("invalid request") // hide details from the client
		}
		if matches.(types.Bool) {
			rejectMessages = append(rejectMessages, rule.reject)
		}
	}

	if len(rejectMessages) > 0 {
		return &admissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result:  &status{Message: strings.Join(rejectMessages, ", ")},
		}, nil
	}
	return &admissionResponse{
		UID:     req.UID,
		Allowed: true,
	}, nil
}
