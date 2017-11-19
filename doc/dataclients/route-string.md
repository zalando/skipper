# Route String

Route string dataclient can be used to create simple demo
applications, for example if you want to show traffic switching or
ratelimiting in your demo.

## Serve text

Serve with `Content-Type: text/plain; charset=utf-8`

Example (Open your browser http://localhost:9090/):

     $ skipper -inline-routes '* -> inlineContent("Hello, world!") -> <shunt>'

Docker Example (Open your browser http://localhost:9090/):

     $ docker run -p 9090:9090 -it registry.opensource.zalan.do/pathfinder/skipper:latest skipper -inline-routes '* -> inlineContent("Hello, world!") -> <shunt>'

## Serve HTML with CSS

Serve with `Content-Type: text/html; charset=utf-8`

Example (Open your browser http://localhost:9090/):

     $ skipper -inline-routes '* -> inlineContent("<html><body style=\"background-color: orange;\"></body></html>") -> <shunt>'

Docker Example (Open your browser http://localhost:9090/):

     $ docker run -p 9090:9090 -it registry.opensource.zalan.do/pathfinder/skipper:latest skipper -inline-routes '* -> inlineContent("<html><body style=\"background-color: orange;\"></body></html>") -> <shunt>'


## Serve JSON

Serve with `Content-Type: application/json; charset=utf-8`

Example (Open your browser http://localhost:9090/):

    % skipper -inline-routes '* -> setResponseHeader("Content-Type", "application/json; charset=utf-8") -> inlineContent("{\"foo\": 3}") -> <shunt>'

Docker Example (Open your browser http://localhost:9090/):

     $ docker run -p 9090:9090 -it registry.opensource.zalan.do/pathfinder/skipper:latest skipper -inline-routes '* -> setResponseHeader("Content-Type", "application/json; charset=utf-8") -> inlineContent("{\"foo\": 3}") -> <shunt>'
