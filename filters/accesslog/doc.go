/*
Package accesslog provides a request filter that gives ability to override AccessLogDisabled setting.

How It Works

The filter accepts one argument of "true" or "false" value. If the argument value is "true" access log entries for this
route won't be produced even if global AccessLogDisabled is false. Otherwise, if argument value is "false" access log
entries for this route will be produced even if global AccessLogDisabled is true.

Usage

	accessLogDisabled("true")
	accessLogDisabled("false")
*/
package accesslog
