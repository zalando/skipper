package filters

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func StringArg(x interface{}) (string, error) {
	if s, ok := x.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("%v is not a string", x)
}

func Float64Arg(x interface{}) (float64, error) {
	switch f := x.(type) {
	case float64:
		return f, nil
	case int:
		return float64(f), nil
	}
	return 0, fmt.Errorf("%v is not a float64", x)
}

func IntArg(x interface{}) (int, error) {
	switch i := x.(type) {
	case int:
		return i, nil
	case float64:
		ii := int(i)
		// check if integer
		if float64(ii) == i {
			return ii, nil
		}
	}
	return 0, fmt.Errorf("%v is not an integer", x)
}

func Int64Arg(x interface{}) (int64, error) {
	switch i := x.(type) {
	case int64:
		return i, nil
	case int:
		return int64(i), nil
	case float64:
		ii := int64(i)
		// check if integer
		if float64(ii) == i {
			return ii, nil
		}
	}
	return 0, fmt.Errorf("%v is not an int64", x)
}

func DurationArg(x interface{}) (time.Duration, error) {
	var d time.Duration
	switch t := x.(type) {
	case time.Duration:
		d = t
	case int:
		d = time.Duration(time.Duration(t) * time.Millisecond)
	case float64:
		// support fractional milliseconds
		d = time.Duration(t * float64(time.Millisecond))
	case string:
		var err error
		d, err = time.ParseDuration(t)
		if err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("%v is not a duration", x)
	}
	if d < 0 {
		return 0, fmt.Errorf("duration %v is negative", x)
	}
	return d, nil
}

type FilterArgs struct {
	args []interface{}
	pos  int
	errs []error
}

// Creates filter arguments wrapper that provides methods
// to sequentially access and convert arguments.
// Every call of non-optional accessor method increases expected argument counter.
// The Err() method returns non nil error if expected argument counter
// does not match input argument array length or if there were conversion errors.
//
// Example usage:
//  a := Args([]interface{}{"s", 1, time.Millisecond})
//  s, i, d, opt, err := a.String(), a.Int(), a.Duration(), a.StringOr("default"), a.Err()
//  if err != nil {
//      return err
//  }
func Args(args []interface{}) *FilterArgs {
	return &FilterArgs{args: args}
}

func (a *FilterArgs) String() (_ string) {
	if x, ok := a.next(); ok {
		if s, err := StringArg(x); err == nil {
			return s
		} else {
			a.error(err)
		}
	}
	return
}

func (a *FilterArgs) StringOr(defaultValue string) string {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.String()
}

func (a *FilterArgs) Strings() (result []string) {
	if a.pos > len(a.args) {
		return nil
	}
	hasErr := false
	for _, x := range a.args[a.pos:] {
		a.pos++
		if s, err := StringArg(x); err == nil {
			result = append(result, s)
		} else {
			a.error(err)
			hasErr = true
		}
	}
	if hasErr {
		return nil
	}
	return
}

func (a *FilterArgs) Float64() (_ float64) {
	if x, ok := a.next(); ok {
		if f, err := Float64Arg(x); err == nil {
			return f
		} else {
			a.error(err)
		}
	}
	return
}

func (a *FilterArgs) Int64() (_ int64) {
	if x, ok := a.next(); ok {
		if i, err := Int64Arg(x); err == nil {
			return i
		} else {
			a.error(err)
		}
	}
	return
}

func (a *FilterArgs) Int() (_ int) {
	if x, ok := a.next(); ok {
		if i, err := IntArg(x); err == nil {
			return i
		} else {
			a.error(err)
		}
	}
	return
}

func (a *FilterArgs) IntOr(defaultValue int) int {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.Int()
}

func (a *FilterArgs) Duration() (_ time.Duration) {
	if x, ok := a.next(); ok {
		if d, err := DurationArg(x); err == nil {
			return d
		} else {
			a.error(err)
		}
	}
	return
}

func (a *FilterArgs) DurationOr(defaultValue time.Duration) time.Duration {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.Duration()
}

func (a *FilterArgs) Err() error {
	var errs []string
	if a.pos != len(a.args) {
		if a.pos == 1 {
			errs = append(errs, "expects 1 argument")
		} else {
			errs = append(errs, fmt.Sprintf("expects %d arguments", a.pos))
		}
	}
	for _, err := range a.errs {
		errs = append(errs, err.Error())
	}

	if len(errs) == 0 {
		return nil
	} else {
		return errors.New(strings.Join(errs, ", "))
	}
}

func (a *FilterArgs) next() (x interface{}, ok bool) {
	if a.pos >= len(a.args) {
		x, ok = nil, false
	} else {
		x, ok = a.args[a.pos], true
	}
	a.pos++
	return
}

func (a *FilterArgs) error(err error) {
	a.errs = append(a.errs, err)
}
