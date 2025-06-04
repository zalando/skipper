package policy

default allow = false

allow if {
    input.attributes.request.http.method == "GET"
    input.attributes.request.http.path == "/authorize"
}