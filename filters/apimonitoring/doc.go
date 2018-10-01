/*

API Monitoring - apimonitoring() filter

The `apimonitoring` filter adds API related metrics to the monitoring.

	a: Path("/this-is-monitored")
		-> apimonitoring(`{
	         "apis": [
	           {
	             "application_id": "my_app",
	             "id": "orders_api",
	             "path_templates": [
	               "foo/orders",
	               "foo/orders/:order-id",
	               "foo/orders/:order-id/order-items/{order-item-id}"
	             ]
	           },
	           {
	             "id": "customers_api",
	             "application_id": "my_app",
	             "path_templates": [
	               "/foo/customers/",
	               "/foo/customers/{customer-id}/"
	             ]
	           }
	         ]
	       }`)
		-> "https://example.org/";



Development Helpers

Command line for executing locally:

make skipper && \
  ./bin/skipper \
    -routes-file "~/temp/test.eskip" \
    -enable-apimonitoring \
    -apimonitoring-verbose \
    -enable-prometheus-metrics \
    -histogram-metric-buckets=".01,.025,.05,.075,.1,.2,.3,.4,.5,.75,1,2,3,4,5,7,10,15,20,30,60,120,300,600"

*/
package apimonitoring
