/*

Package apimonitoring provides filters gathering metrics around API calls

Feature switch & Dependencies

This feature is considered experimental and should only be activated explicitly. To
enable it, the flag `-enable-apimonitoring` must be set when Skipper is launched. Also,
it does not make sense if none of the metrics implementation is enabled. To use this
filter, start skipper enabling both. Per instance:

	skipper -enable-apimonitoring -enable-prometheus-metrics

This will enable the API monitoring filter through Prometheus metrics.


Configuration

Due to its structured configuration, the filter accepts one parameter of type string
containing a JSON object.

Details can be found at https://opensource.zalando.com/skipper/reference/filters/#apimonitoring
(or in this project, under `docs/reference/filters.md`).

Example:

	Path("/this-is-monitored")
		-> apimonitoring(`{
				"application_id": "my_app",
				"path_templates": [
					"foo/orders",
					"foo/orders/:order-id",
					"foo/orders/:order-id/order-items/{order-item-id}"
					"/foo/customers/",
					"/foo/customers/{customer-id}/"
				]
			}`)
		-> "https://example.org/";


Development Helpers

Command line example for executing locally:

	make skipper && \
	  ./bin/skipper \
		-routes-file "~/temp/test.eskip" \
		-enable-apimonitoring \
		-apimonitoring-verbose \
		-enable-prometheus-metrics \
		-histogram-metric-buckets=".01,.025,.05,.075,.1,.2,.3,.4,.5,.75,1,2,3,4,5,7,10,15,20,30,60,120,300,600"

*/
package apimonitoring
