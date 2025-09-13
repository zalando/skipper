/*
Package awssigv4 signs requests using aws signature version 4 mechanism. see https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-authenticating-requests.html

# Filter awsSigv4
awsSigv4 filter can be defined on a route as `awsSigv4("<service>, "<region>", <DisableHeaderHoisting>, <DisableURIPathEscaping>, <DisableSessionToken>)`

An example of route with awsSigv4 filter is

		`editorRoute: * -> awsSigv4("dynamodb" , "us-east-1", false, false, false) -> "https://dynamodb.us-east-1.amazonaws.com";`

		This filter expects
		- Service
			An aws service name. Please refer valid service names from service endpoint.
			For example if service endpoint is https://dynamodb.us-east-1.amazonaws.com, then service is dynamodb

		- Region
			AWS region where service is located. Please refer valid service names from service endpoint.
			For example if service endpoint is https://dynamodb.us-east-1.amazonaws.com, then region is us-east-1.

		- DisableHeaderHoisting
			Disables the Signer's moving HTTP header key/value pairs from the HTTP request header to the request's query string. This is most commonly used
			with pre-signed requests preventing headers from being added to the request's query string.

		- DisableURIPathEscaping
			Disables the automatic escaping of the URI path of the request for the siganture's canonical string's path. For services that do not need additional
			escaping then use this to disable the signer escaping the path. S3 is an example of a service that does not need additional escaping.
			http://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html

		- DisableSessionToken
			Disables setting the session token on the request as part of signing through X-Amz-Security-Token. This is needed for variations of v4 that
			present the token elsewhere.

		The filter also expects following headers to be present to sign the request
			- x-amz-accesskey
				A valid AWS access key
	     	- x-amz-secret
				A valid AWS secret
			- x-amz-session
				A valid session token  [see](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_use-resources.html#using-temp-creds-sdk)
			- x-amz-time
				A RFC3339 format time stamp which should be considered as time stamp of signing the request.

		Filter removes these headers after reading the values and does not alter the signature produced. Once the signature is generated, it is appended to Authorization header and forwarded to AWS service.

# Memory consideration
This filter reads the body in memory. This is needed to generate signature as per Signature V4 specs. Special considerations need to be taken when operating the skipper with concurrent requests.

# Overwriting io.ReadCloser
This filter resets `read` and `close` implementations of body to default. So in case a filter before this filter has some custom implementations of thse methods, they would be overwritten.
*/
package awssigv4
