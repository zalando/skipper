package humanstxt

import (
	"io"
	"skipper/middleware/noop"
	"skipper/skipper"
	"strconv"
)

const name = "humans.txt"

const text = `
                         $$$$$$$$$$$
                      $$$;;;;;;;;;;;$$$
                     $;;;;;;;;;;;;;;;;$$
                    $;;;;;;;;;;;;;;;;;;;$
                   $;;;;;;;;;;;;;;;;;;;;;$
                  $;;;;;;;;;;;;;;;;;;;;;;;$
                 $;;;;;;;;;;;;;;;;;;;;;;;;;$
                 $;;;;;;;;//%%+%%//;;;;;;;;;$
                $;;;;;;;/$HH$:.:$HH$/;;;;;;;$
                $;;;;;;$H$$%.....%$$H$;;;;;;;$
               $;;;;;;$H$$/......./$$$H/;;;;;$
               $;;;;;%H$$%+++....+++$$$%;;;;;$$
               $;;;;/H$$$.   +..+  .%$$H/;;;;;$
               $;;;;%$$$/     +     :$$H%;;;;;$
               $;;;/%$$$-     +.    .H$$%/;;;;$
               $;;;/%$$H,   - .. -   $H$$/;;;;$
               $;;;/%$$$.  -# +. #-  %$$$/;;;;$
               $;;;/%$$$=     +     .$$$%/;;;;$
               $;;;/%H$$/    .++    -$$H%/;;;;$
               $;;;//$$$$,   +..+   /$$H%/;;;;$
               $$;;;/X$$$+..+....+ =$$$X//;;;$
                $;;;/%$$$$%+......+$$H$%/;;;;$ 
                $;;;;/%$$$$%......$$$$X//;;;;$
                $$;;;//%X$$$$/../$$$$%//;;;;$$
                 $;;;;///+%X$XMMHX%%///;;;;/$
                  $;;;;;//;;%H###$///;;;;;;$
                   $;;;;;//+H#$%##//;;;;;;$
                    $;;;;;;;#@//%#%;;;;;;$
                     $;;;;;/M/;;;%$;;;;/$
                      $$/;;;/;;;;;;;;;$$
                        $$$;;;;;;;;$$$
                          $$$$$$$$$$
`

type humanstxt struct {
	*noop.Type
}

func Make() skipper.Middleware {
	return &humanstxt{}
}

func (h *humanstxt) Name() string {
	return name
}

func (h *humanstxt) MakeFilter(id string, _ skipper.MiddlewareConfig) (skipper.Filter, error) {
	hf := &humanstxt{&noop.Type{}}
	hf.SetId(id)
	return hf, nil
}

func (h *humanstxt) Response(ctx skipper.FilterContext) {
	r := ctx.Response()
	println("response", r == nil)
	r.StatusCode = 200
	r.Header.Set("Content-Type", "text/plain")
	println("response header", r.Header == nil)
	r.Header.Set("Content-Length", strconv.Itoa(len(text)))
	println("response body", r.Body == nil)
	println("response body as writer", r.Body.(io.Writer) == nil)
	r.Body.(io.Writer).Write([]byte(text))
}
