/*
Package accesslog provides request filters that give the ability to override AccessLogDisabled setting.

How It Works

There are two filters that change the state of access log "disableAccessLog" and "enableAccessLog". If "disableAccessLog" is
present access log entries for this route won't be produced even if global AccessLogDisabled is false. Otherwise, if
"enableAccessLog" filter is present access log entries for this route will be produced even if global AccessLogDisabled
is true.

Usage

    enableAccessLog()
    disableAccessLog()

Note: accessLogDisabled("true") filter is deprecated in favor of "disableAccessLog" and "enableAccessLog"
*/
package accesslog
