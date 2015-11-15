// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package filters contains definitions for skipper filtering and a default,
built-in set of filters.

Filters are used to augment both the inbound request's attributes before
forwarding it to the route endpoint, and the outbound response's
attributes before returning it to the original client.


Filter Specification and Filter Instances

Filter implementations are based on filter specifications that provide a
filter name and a 'factory' method to create filter instances. The filter name
is used to identify a filter in a route definition. The filter specifications
can be used by multiple routes, while the filter instances belong to a single
route. Filter instances are created while the route definitions are parsed and
initialized, based on the specifications stored in the filter registry.
Different filter instances can be created with different parameters.


Filtering and FilterContext

Once a route is identified during request processing, a context object is
created that is unique to the request, holding the current request, the
response (once it is available), and some further information and state
related to the current flow.

Each filter in a route is called twice, once for the request in the order of
their position in the route definition, and once for the response in reverse
order.


Handling Requests with Filters

Filters can handle the requests themselves, meaning that they can set the
response status, headers and send any particular response body. In this case,
it is the filter's responsibility to mark the request as served to avoid
generating the default response.
*/
package filters
