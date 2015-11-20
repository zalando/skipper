/*
Package logging implements application log instrumentation and Apache
combined access log.

Application Log

The application log uses the logrus package:

https://github.com/Sirupsen/logrus

To send messages to the application log, import this package and use its
methods. Example:

    import log "github.com/Sirupsen/logrus"

    func doSomething() {
        log.Errorf("nothing to do")
    }

During startup initialization, it is possible to redirect the log output
from the default /dev/stderr to another file, and to set a common
prefix for each log entry. Setting the prefix may be a good idea when
the access log is enabled and its output is the same as the one of the
application log, to make it easier to split the output for diagnostics.

Access Log

The access log prints HTTP access information in the Apache combined
access log format. To output entries, use the logging.Access method.
Note that by default, skipper uses the loggingHandler to wrap the
central proxy handler, and automatically provides access logging.

During initialization, it is possible to redirect the access log output
from the default /dev/stderr to another file, or completely disable the
access log.

Output Files

To set a custom file output for the application log or the access log is
currently not recommended in production environment, because neither the
proper handling of system errors, or a log rolling mechanism is
implemented at the current stage.
*/
package logging
