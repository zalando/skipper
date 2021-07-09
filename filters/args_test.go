package filters

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestArgsStringArg(t *testing.T) {
	s, err := StringArg("s")
	assert.Nil(t, err)
	assert.Equal(t, "s", s)

	s, err = StringArg("")
	assert.Nil(t, err)
	assert.Equal(t, "", s)

	s, err = StringArg(1)
	assert.EqualError(t, err, "1 is not a string")
	assert.Equal(t, "", s)

	s, err = StringArg(1.1)
	assert.EqualError(t, err, "1.1 is not a string")
	assert.Equal(t, "", s)

	s, err = StringArg(nil)
	assert.EqualError(t, err, "<nil> is not a string")
	assert.Equal(t, "", s)
}

func TestArgsFloat64Arg(t *testing.T) {
	f, err := Float64Arg(1.)
	assert.Nil(t, err)
	assert.Equal(t, 1., f)

	f, err = Float64Arg(1)
	assert.Nil(t, err)
	assert.Equal(t, 1., f)

	f, err = Float64Arg(1.5)
	assert.Nil(t, err)
	assert.Equal(t, 1.5, f)

	f, err = Float64Arg("1")
	assert.EqualError(t, err, "1 is not a float64")
	assert.Equal(t, 0., f)

	f, err = Float64Arg(nil)
	assert.EqualError(t, err, "<nil> is not a float64")
	assert.Equal(t, 0., f)
}

func TestArgsIntArg(t *testing.T) {
	i, err := IntArg(1)
	assert.Nil(t, err)
	assert.Equal(t, 1, i)

	i, err = IntArg(1.0)
	assert.Nil(t, err)
	assert.Equal(t, 1, i)

	i, err = IntArg(1.5)
	assert.EqualError(t, err, "1.5 is not an integer")
	assert.Equal(t, 0, i)

	i, err = IntArg("1")
	assert.EqualError(t, err, "1 is not an integer")
	assert.Equal(t, 0, i)

	i, err = IntArg(nil)
	assert.EqualError(t, err, "<nil> is not an integer")
	assert.Equal(t, 0, i)
}

func TestArgsInt64Arg(t *testing.T) {
	i, err := Int64Arg(1)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), i)

	i, err = Int64Arg(1.0)
	assert.Nil(t, err)
	assert.Equal(t, int64(1), i)

	i, err = Int64Arg(1.5)
	assert.EqualError(t, err, "1.5 is not an int64")
	assert.Equal(t, int64(0), i)

	i, err = Int64Arg("1")
	assert.EqualError(t, err, "1 is not an int64")
	assert.Equal(t, int64(0), i)

	i, err = Int64Arg(nil)
	assert.EqualError(t, err, "<nil> is not an int64")
	assert.Equal(t, int64(0), i)
}

func TestArgsDurationArg(t *testing.T) {
	d, err := DurationArg("11ms")
	assert.Nil(t, err)
	assert.Equal(t, 11*time.Millisecond, d)

	d, err = DurationArg(12 * time.Millisecond)
	assert.Nil(t, err)
	assert.Equal(t, 12*time.Millisecond, d)

	d, err = DurationArg("-1ms")
	assert.EqualError(t, err, "duration -1ms is negative")
	assert.Equal(t, time.Duration(0), d)

	d, err = DurationArg("ms")
	assert.EqualError(t, err, `time: invalid duration "ms"`)
	assert.Equal(t, time.Duration(0), d)

	d, err = DurationArg(11)
	assert.EqualError(t, err, "11 is not a duration")
	assert.Equal(t, time.Duration(0), d)

	d, err = DurationArg(1.1)
	assert.EqualError(t, err, "1.1 is not a duration")
	assert.Equal(t, time.Duration(0), d)

	d, err = DurationArg(nil)
	assert.EqualError(t, err, "<nil> is not a duration")
	assert.Equal(t, time.Duration(0), d)
}

func TestArgsDurationOrNumberArg(t *testing.T) {
	d, err := DurationOrNumberArg(11, time.Second)
	assert.Nil(t, err)
	assert.Equal(t, 11*time.Second, d)

	d, err = DurationOrNumberArg(1.1, time.Millisecond)
	assert.Nil(t, err)
	assert.Equal(t, 1100*time.Microsecond, d)

	d, err = DurationOrNumberArg(0.5, time.Millisecond)
	assert.Nil(t, err)
	assert.Equal(t, 500*time.Microsecond, d)

	d, err = DurationOrNumberArg("1ms", time.Second)
	assert.Nil(t, err)
	assert.Equal(t, 1*time.Millisecond, d)

	d, err = DurationOrNumberArg(1*time.Millisecond, time.Second)
	assert.Nil(t, err)
	assert.Equal(t, 1*time.Millisecond, d)

	d, err = DurationOrNumberArg("-1ms", time.Second)
	assert.EqualError(t, err, "duration -1ms is negative")
	assert.Equal(t, time.Duration(0), d)

	d, err = DurationOrNumberArg("ms", time.Second)
	assert.EqualError(t, err, `time: invalid duration "ms"`)
	assert.Equal(t, time.Duration(0), d)

	d, err = DurationOrNumberArg(nil, time.Second)
	assert.EqualError(t, err, "<nil> is not a duration")
	assert.Equal(t, time.Duration(0), d)
}

func TestArgsString(t *testing.T) {
	a := Args([]interface{}{"s0", "s1"})
	s0, s1 := a.String(), a.String()

	assert.Nil(t, a.Err())
	assert.Equal(t, "s0", s0)
	assert.Equal(t, "s1", s1)

	a = Args([]interface{}{"s0", "s1", 123.4})
	_, _, s2 := a.String(), a.String(), a.String()

	assert.EqualError(t, a.Err(), "123.4 is not a string")
	assert.Equal(t, "", s2)

	a = Args([]interface{}{"s0", "s1", 123.4})
	_, _, s2, s3 := a.String(), a.String(), a.String(), a.String()

	assert.EqualError(t, a.Err(), "expects 4 arguments, 123.4 is not a string")
	assert.Equal(t, "", s2)
	assert.Equal(t, "", s3)
}

func TestArgsEmpty(t *testing.T) {
	a := Args([]interface{}{})
	s := a.String()

	assert.EqualError(t, a.Err(), "expects 1 argument")
	assert.Equal(t, "", s)
}

func TestArgsOptionalString(t *testing.T) {
	a := Args([]interface{}{"s0"})
	s0 := a.OptionalString("x")

	assert.Nil(t, a.Err())
	assert.Equal(t, "s0", s0)

	a = Args([]interface{}{"s0", 123.4})
	_, s1 := a.OptionalString("x"), a.OptionalString("y")

	assert.EqualError(t, a.Err(), "123.4 is not a string")
	assert.Equal(t, "", s1)

	a = Args([]interface{}{"s0", 123.4})
	_, _, s2, s3 := a.String(), a.Float64(), a.OptionalString("z"), a.OptionalString("Z")

	assert.Nil(t, a.Err())
	assert.Equal(t, "z", s2)
	assert.Equal(t, "Z", s3)
}

func TestArgsStrings(t *testing.T) {
	a := Args([]interface{}{"s1", "s2", "s3"})
	s0 := a.Strings()

	assert.Nil(t, a.Err())
	assert.Equal(t, []string{"s1", "s2", "s3"}, s0)

	a = Args([]interface{}{"s1"})
	s0 = a.Strings()

	assert.Nil(t, a.Err())
	assert.Equal(t, []string{"s1"}, s0)

	a = Args([]interface{}{})
	s0 = a.Strings()

	assert.Nil(t, a.Err())
	assert.Nil(t, s0)

	a = Args([]interface{}{123.4, "s1", "s2"})
	_, s1 := a.Float64(), a.Strings()

	assert.Nil(t, a.Err())
	assert.Equal(t, []string{"s1", "s2"}, s1)

	a = Args([]interface{}{123.4, "s1", "s2"})
	_, _, s2 := a.Float64(), a.String(), a.Strings()

	assert.Nil(t, a.Err())
	assert.Equal(t, []string{"s2"}, s2)

	a = Args([]interface{}{123.4, "s1", "s2"})
	_, _, _, s3 := a.Float64(), a.String(), a.String(), a.Strings()

	assert.Nil(t, a.Err())
	assert.Nil(t, s3)

	a = Args([]interface{}{123.4, "s1", "s2"})
	_, _, _, _, s4 := a.Float64(), a.String(), a.String(), a.String(), a.Strings()

	assert.EqualError(t, a.Err(), "expects 4 arguments")
	assert.Nil(t, s4)

	a = Args([]interface{}{123.4, "s1", "s2", 5.67})
	s0 = a.Strings()
	assert.EqualError(t, a.Err(), "123.4 is not a string, 5.67 is not a string")
	assert.Nil(t, s0)
}

func TestArgsFloat64(t *testing.T) {
	a := Args([]interface{}{1., 1.2, 2})
	f0, f1, f2 := a.Float64(), a.Float64(), a.Float64()

	assert.Nil(t, a.Err())
	assert.Equal(t, 1., f0)
	assert.Equal(t, 1.2, f1)
	assert.Equal(t, 2., f2) // from int

	a = Args([]interface{}{1., 1.2, "s"})
	_, _, f2 = a.Float64(), a.Float64(), a.Float64()

	assert.EqualError(t, a.Err(), "s is not a float64")
	assert.Equal(t, 0., f2)

	a = Args([]interface{}{1., 1.2, "s"})
	_, _, _, f3 := a.Float64(), a.Float64(), a.Float64(), a.Float64()

	assert.EqualError(t, a.Err(), "expects 4 arguments, s is not a float64")
	assert.Equal(t, 0., f3)
}

func TestArgsOptionalFloat64(t *testing.T) {
	a := Args([]interface{}{10})
	f0 := a.OptionalFloat64(100)

	assert.Nil(t, a.Err())
	assert.Equal(t, 10., f0)

	a = Args([]interface{}{10, 11.})
	f0, f1, f2, f3 := a.Float64(), a.OptionalFloat64(100), a.OptionalFloat64(12), a.OptionalFloat64(13)

	assert.Nil(t, a.Err())
	assert.Equal(t, 10., f0)
	assert.Equal(t, 11., f1)
	assert.Equal(t, 12., f2)
	assert.Equal(t, 13., f3)

	a = Args([]interface{}{10, "x"})
	_, f1 = a.Float64(), a.OptionalFloat64(100)

	assert.EqualError(t, a.Err(), "x is not a float64")
	assert.Equal(t, 0., f1)
}

func TestArgsInt64(t *testing.T) {
	a := Args([]interface{}{1., 2, int64(3)})
	i0, i1, i2 := a.Int64(), a.Int64(), a.Int64()

	assert.Nil(t, a.Err())
	assert.Equal(t, int64(1), i0) // from float64
	assert.Equal(t, int64(2), i1)
	assert.Equal(t, int64(3), i2)

	a = Args([]interface{}{1.2, "s"})
	i0, i1, i2 = a.Int64(), a.Int64(), a.Int64()

	assert.EqualError(t, a.Err(), "expects 3 arguments, 1.2 is not an int64, s is not an int64")
	assert.Equal(t, int64(0), i0)
	assert.Equal(t, int64(0), i1)
	assert.Equal(t, int64(0), i2)
}

func TestArgsOptionalInt64(t *testing.T) {
	a := Args([]interface{}{10})
	i0 := a.OptionalInt64(100)

	assert.Nil(t, a.Err())
	assert.Equal(t, int64(10), i0)

	a = Args([]interface{}{10, int64(11)})
	i0, i1, i2, i3 := a.Int64(), a.OptionalInt64(100), a.OptionalInt64(12), a.OptionalInt64(13)

	assert.Nil(t, a.Err())
	assert.Equal(t, int64(10), i0)
	assert.Equal(t, int64(11), i1)
	assert.Equal(t, int64(12), i2)
	assert.Equal(t, int64(13), i3)

	a = Args([]interface{}{10, "x"})
	_, i1 = a.Int64(), a.OptionalInt64(100)

	assert.EqualError(t, a.Err(), "x is not an int64")
	assert.Equal(t, int64(0), i1)
}

func TestArgsInt(t *testing.T) {
	a := Args([]interface{}{1, 2.})
	i0, i1 := a.Int(), a.Int()

	assert.Nil(t, a.Err())
	assert.Equal(t, 1, i0)
	assert.Equal(t, 2, i1) // from float64

	a = Args([]interface{}{1., 1.2})
	_, i1 = a.Int(), a.Int()

	assert.EqualError(t, a.Err(), "1.2 is not an integer")
	assert.Equal(t, 0, i1)

	a = Args([]interface{}{1., 1.2, "s"})
	_, i1, i2 := a.Int(), a.Int(), a.Int()

	assert.EqualError(t, a.Err(), "1.2 is not an integer, s is not an integer")
	assert.Equal(t, 0, i1)
	assert.Equal(t, 0, i2)

	a = Args([]interface{}{1., 1.2, "s"})
	_, _, _, i3 := a.Int(), a.Int(), a.Int(), a.Int()

	assert.EqualError(t, a.Err(), "expects 4 arguments, 1.2 is not an integer, s is not an integer")
	assert.Equal(t, 0, i3)
}

func TestArgsOptionalInt(t *testing.T) {
	a := Args([]interface{}{10})
	i0 := a.OptionalInt(100)

	assert.Nil(t, a.Err())
	assert.Equal(t, 10, i0)

	a = Args([]interface{}{10, 11})
	i0, i1, i2, i3 := a.Int(), a.OptionalInt(100), a.OptionalInt(12), a.OptionalInt(13)

	assert.Nil(t, a.Err())
	assert.Equal(t, 10, i0)
	assert.Equal(t, 11, i1)
	assert.Equal(t, 12, i2)
	assert.Equal(t, 13, i3)

	a = Args([]interface{}{10, "x"})
	_, i1 = a.Int(), a.OptionalInt(100)

	assert.EqualError(t, a.Err(), "x is not an integer")
	assert.Equal(t, 0, i1)
}

func TestArgsDuration(t *testing.T) {
	a := Args([]interface{}{"123s", 11 * time.Microsecond})
	d0, d1 := a.Duration(), a.Duration()

	assert.Nil(t, a.Err())
	assert.Equal(t, 123*time.Second, d0)
	assert.Equal(t, 11*time.Microsecond, d1)

	a = Args([]interface{}{"-123s", 10, 12.3, "whatever", -11 * time.Microsecond})
	d0, d1, d2, d3, d4 := a.Duration(), a.Duration(), a.Duration(), a.Duration(), a.Duration()

	assert.EqualError(t, a.Err(), `duration -123s is negative, `+
		`10 is not a duration, `+
		`12.3 is not a duration, `+
		`time: invalid duration "whatever", `+
		`duration -11µs is negative`)
	assert.Equal(t, time.Duration(0), d0)
	assert.Equal(t, time.Duration(0), d1)
	assert.Equal(t, time.Duration(0), d2)
	assert.Equal(t, time.Duration(0), d3)
	assert.Equal(t, time.Duration(0), d4)
}

func TestArgsOptionalDuration(t *testing.T) {
	a := Args([]interface{}{"10s"})
	d0 := a.OptionalDuration(10 * time.Minute)

	assert.Nil(t, a.Err())
	assert.Equal(t, 10*time.Second, d0)

	a = Args([]interface{}{11 * time.Second})
	d0 = a.OptionalDuration(10 * time.Minute)

	assert.Nil(t, a.Err())
	assert.Equal(t, 11*time.Second, d0)

	a = Args([]interface{}{12})
	d0 = a.OptionalDuration(10 * time.Minute)

	assert.EqualError(t, a.Err(), `12 is not a duration`)
	assert.Equal(t, time.Duration(0), d0)

	a = Args([]interface{}{"10s", "x"})
	_, d1 := a.Duration(), a.OptionalDuration(10*time.Minute)

	assert.EqualError(t, a.Err(), `time: invalid duration "x"`)
	assert.Equal(t, time.Duration(0), d1)

	a = Args([]interface{}{"10s"})
	d0, d1, d2, d3 := a.Duration(), a.OptionalDuration(11*time.Second), a.OptionalDuration(12*time.Second), a.OptionalDuration(13*time.Second)

	assert.Nil(t, a.Err())
	assert.Equal(t, 10*time.Second, d0)
	assert.Equal(t, 11*time.Second, d1)
	assert.Equal(t, 12*time.Second, d2)
	assert.Equal(t, 13*time.Second, d3)
}

func TestArgsDurationOrMilliseconds(t *testing.T) {
	a := Args([]interface{}{"123s", 10, 12.3, 0.5, 11 * time.Microsecond})
	d0, d1, d2, d3, d4 := a.DurationOrMilliseconds(), a.DurationOrMilliseconds(), a.DurationOrMilliseconds(), a.DurationOrMilliseconds(), a.DurationOrMilliseconds()

	assert.Nil(t, a.Err())
	assert.Equal(t, 123*time.Second, d0)
	assert.Equal(t, 10*time.Millisecond, d1)
	assert.Equal(t, 12300*time.Microsecond, d2)
	assert.Equal(t, 500*time.Microsecond, d3)
	assert.Equal(t, 11*time.Microsecond, d4)

	a = Args([]interface{}{"-123s", -10, -12.3, "whatever", -11 * time.Microsecond})
	d0, d1, d2, d3, d4 = a.DurationOrMilliseconds(), a.DurationOrMilliseconds(), a.DurationOrMilliseconds(), a.DurationOrMilliseconds(), a.DurationOrMilliseconds()

	assert.EqualError(t, a.Err(), `duration -123s is negative, `+
		`duration -10 is negative, `+
		`duration -12.3 is negative, `+
		`time: invalid duration "whatever", `+
		`duration -11µs is negative`)
	assert.Equal(t, time.Duration(0), d0)
	assert.Equal(t, time.Duration(0), d1)
	assert.Equal(t, time.Duration(0), d2)
	assert.Equal(t, time.Duration(0), d3)
	assert.Equal(t, time.Duration(0), d4)
}

func TestArgsOptionalDurationOrMilliseconds(t *testing.T) {
	a := Args([]interface{}{"10s"})
	d0 := a.OptionalDurationOrMilliseconds(10 * time.Minute)

	assert.Nil(t, a.Err())
	assert.Equal(t, 10*time.Second, d0)

	a = Args([]interface{}{11})
	d0 = a.OptionalDurationOrMilliseconds(10 * time.Minute)

	assert.Nil(t, a.Err())
	assert.Equal(t, 11*time.Millisecond, d0)

	a = Args([]interface{}{"10s", "x"})
	_, d1 := a.Duration(), a.OptionalDurationOrMilliseconds(10*time.Minute)

	assert.EqualError(t, a.Err(), `time: invalid duration "x"`)
	assert.Equal(t, time.Duration(0), d1)

	a = Args([]interface{}{"10s"})
	d0, d1, d2, d3 := a.Duration(), a.OptionalDurationOrMilliseconds(11*time.Second), a.OptionalDurationOrMilliseconds(12*time.Second), a.OptionalDurationOrMilliseconds(13*time.Second)

	assert.Nil(t, a.Err())
	assert.Equal(t, 10*time.Second, d0)
	assert.Equal(t, 11*time.Second, d1)
	assert.Equal(t, 12*time.Second, d2)
	assert.Equal(t, 13*time.Second, d3)
}

func TestArgsDurationOrSeconds(t *testing.T) {
	a := Args([]interface{}{"123s", 10, 12.3, 0.5, 11 * time.Microsecond})
	d0, d1, d2, d3, d4 := a.DurationOrSeconds(), a.DurationOrSeconds(), a.DurationOrSeconds(), a.DurationOrSeconds(), a.DurationOrSeconds()

	assert.Nil(t, a.Err())
	assert.Equal(t, 123*time.Second, d0)
	assert.Equal(t, 10*time.Second, d1)
	assert.Equal(t, 12300*time.Millisecond, d2)
	assert.Equal(t, 500*time.Millisecond, d3)
	assert.Equal(t, 11*time.Microsecond, d4)

	a = Args([]interface{}{"-123s", -10, -12.3, "whatever", -11 * time.Microsecond})
	d0, d1, d2, d3, d4 = a.DurationOrSeconds(), a.DurationOrSeconds(), a.DurationOrSeconds(), a.DurationOrSeconds(), a.DurationOrSeconds()

	assert.EqualError(t, a.Err(), `duration -123s is negative, `+
		`duration -10 is negative, `+
		`duration -12.3 is negative, `+
		`time: invalid duration "whatever", `+
		`duration -11µs is negative`)
	assert.Equal(t, time.Duration(0), d0)
	assert.Equal(t, time.Duration(0), d1)
	assert.Equal(t, time.Duration(0), d2)
	assert.Equal(t, time.Duration(0), d3)
	assert.Equal(t, time.Duration(0), d4)
}

func ExampleArgs() {
	a := Args([]interface{}{"s", 1, time.Millisecond})
	s, i, d, opt, err := a.String(), a.Int(), a.Duration(), a.OptionalString("default"), a.Err()

	fmt.Printf("%#v %#v %#v %#v %#v\n", s, i, d, opt, err)

	a = Args([]interface{}{"s", 1})
	_, _, _, err = a.Int(), a.String(), a.Duration(), a.Err()

	fmt.Println(err.Error())

	// Output:
	// "s" 1 1000000 "default" <nil>
	// expects 3 arguments, s is not an integer, 1 is not a string
}
