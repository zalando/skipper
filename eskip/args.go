package eskip

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func stringArg(x interface{}) (string, error) {
	if s, ok := x.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("%v is not a string", x)
}

func float64Arg(x interface{}) (float64, error) {
	switch f := x.(type) {
	case float64:
		return f, nil
	case int:
		return float64(f), nil
	}
	return 0, fmt.Errorf("%v is not a float64", x)
}

func intArg(x interface{}) (int, error) {
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

func int64Arg(x interface{}) (int64, error) {
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

// Converts string argument into time.Duration using time.ParseDuration.
// Uses time.Duration argument as is.
// Returns error if duration is negative.
func durationArg(x interface{}) (time.Duration, error) {
	var d time.Duration
	switch t := x.(type) {
	case time.Duration:
		d = t
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

// Converts int or float64 argument into time.Duration multiplying by scale argument otherwise delegates to durationArg.
// Returns error if duration is negative.
func durationOrNumberArg(x interface{}, scale time.Duration) (time.Duration, error) {
	var d time.Duration
	switch t := x.(type) {
	case int:
		d = time.Duration(t) * scale
	case float64:
		// convert scale to float64 to support t < 1.0
		d = time.Duration(t * float64(scale))
	default:
		return durationArg(x)
	}
	if d < 0 {
		return 0, fmt.Errorf("duration %v is negative", x)
	}
	return d, nil
}

type EskipArgs struct {
	args []interface{}
	pos  int
	errs []error
}

// Creates arguments wrapper that provides methods
// to sequentially access and convert arguments.
// Every call of non-optional accessor method increases expected argument counter.
// The Err() method returns non nil error if expected argument counter
// does not match input argument array length or if there were conversion errors.
//
// Example usage:
//  a := Args([]interface{}{"s", 1, time.Millisecond})
//  s, i, d, opt, err := a.String(), a.Int(), a.Duration(), a.OptionalString("default"), a.Err()
//  if err != nil {
//      return err
//  }
func Args(args []interface{}) *EskipArgs {
	return &EskipArgs{args: args}
}

func (a *EskipArgs) String() (_ string) {
	if x, ok := a.next(); ok {
		if s, err := stringArg(x); err == nil {
			return s
		} else {
			a.error(err)
		}
	}
	return
}

func (a *EskipArgs) OptionalString(defaultValue string) string {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.String()
}

func (a *EskipArgs) Strings() (result []string) {
	if a.pos > len(a.args) {
		return nil
	}
	hasErr := false
	for _, x := range a.args[a.pos:] {
		a.pos++
		if s, err := stringArg(x); err == nil {
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

func (a *EskipArgs) Float64() (_ float64) {
	if x, ok := a.next(); ok {
		if f, err := float64Arg(x); err == nil {
			return f
		} else {
			a.error(err)
		}
	}
	return
}

func (a *EskipArgs) OptionalFloat64(defaultValue float64) float64 {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.Float64()
}

func (a *EskipArgs) Int64() (_ int64) {
	if x, ok := a.next(); ok {
		if i, err := int64Arg(x); err == nil {
			return i
		} else {
			a.error(err)
		}
	}
	return
}

func (a *EskipArgs) OptionalInt64(defaultValue int64) int64 {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.Int64()
}

func (a *EskipArgs) Int() (_ int) {
	if x, ok := a.next(); ok {
		if i, err := intArg(x); err == nil {
			return i
		} else {
			a.error(err)
		}
	}
	return
}

func (a *EskipArgs) OptionalInt(defaultValue int) int {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.Int()
}

func (a *EskipArgs) Duration() (_ time.Duration) {
	if x, ok := a.next(); ok {
		if d, err := durationArg(x); err == nil {
			return d
		} else {
			a.error(err)
		}
	}
	return
}

func (a *EskipArgs) OptionalDuration(defaultValue time.Duration) time.Duration {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.Duration()
}

// introduced for backwards compatibility, use Duration
func (a *EskipArgs) DurationOrMilliseconds() (_ time.Duration) {
	if x, ok := a.next(); ok {
		if d, err := durationOrNumberArg(x, time.Millisecond); err == nil {
			return d
		} else {
			a.error(err)
		}
	}
	return
}

// introduced for backwards compatibility, use OptionalDuration
func (a *EskipArgs) OptionalDurationOrMilliseconds(defaultValue time.Duration) time.Duration {
	if a.pos >= len(a.args) {
		return defaultValue
	}
	return a.DurationOrMilliseconds()
}

// introduced for backwards compatibility, use Duration
func (a *EskipArgs) DurationOrSeconds() (_ time.Duration) {
	if x, ok := a.next(); ok {
		if d, err := durationOrNumberArg(x, time.Second); err == nil {
			return d
		} else {
			a.error(err)
		}
	}
	return
}

func (a *EskipArgs) Err() error {
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

func (a *EskipArgs) next() (x interface{}, ok bool) {
	if a.pos >= len(a.args) {
		x, ok = nil, false
	} else {
		x, ok = a.args[a.pos], true
	}
	a.pos++
	return
}

func (a *EskipArgs) error(err error) {
	a.errs = append(a.errs, err)
}
