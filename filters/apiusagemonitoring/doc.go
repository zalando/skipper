/*

Package apiusagemonitoring provides filters gathering metrics around API calls


Feature switch & Dependencies

This feature is by default not enabled. To enable it, the flag `-enable-api-usage-monitoring`
must be set when Skipper is launched. Also, it does not make sense if none of the metrics
implementation is enabled. To use this filter, start skipper enabling at least one metrics
flavour. Per instance:

	skipper -enable-api-usage-monitoring -metrics-flavour prometheus

This will enable the API monitoring filter through Prometheus metrics.


Configuration

Due to its structured configuration, the filter accepts one parameter of type string
containing a JSON object.

Details and examples can be found at https://opensource.zalando.com/skipper/reference/filters/#apiUsageMonitoring
(or in this project, under `docs/reference/filters.md`).


Development Helpers

The spec and filter log detailed operation information at `DEBUG` level. The -application-log-level=DEBUG switch
is desirable for debugging usage of the filter.

Command line example for executing locally:

	make skipper && ./bin/skipper \
		-routes-file "$HOME/temp/test.eskip" \
		-enable-api-usage-monitoring \
		-api-usage-monitoring-realm-keys="https://identity.zalando.com/realm" \
		-api-usage-monitoring-client-keys="https://identity.zalando.com/managed-id" \
		-metrics-flavour prometheus \
		-histogram-metric-buckets=.01,.025,.05,.075,.1,.2,.3,.4,.5,.75,1,2,3,4,5,7,10,15,20,30,60,120,300,600 \
		-application-log-level=DEBUG

*/
package apiusagemonitoring
