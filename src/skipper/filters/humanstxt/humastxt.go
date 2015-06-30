package humanstxt

import (
	"io"
	"skipper/filters/noop"
	"skipper/skipper"
	"strconv"
)

const name = "humanstxt"

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

func Make() skipper.FilterSpec {
	return &humanstxt{}
}

func (h *humanstxt) Name() string {
	return name
}

func (h *humanstxt) MakeFilter(id string, _ skipper.FilterConfig) (skipper.Filter, error) {
	hf := &humanstxt{&noop.Type{}}
	hf.SetId(id)
	return hf, nil
}

func (h *humanstxt) Response(ctx skipper.FilterContext) {
	r := ctx.Response()
	r.StatusCode = 200
	r.Header.Set("Content-Type", "text/plain")
	r.Header.Set("Content-Length", strconv.Itoa(len(text)))
	r.Body.(io.Writer).Write([]byte(text))
}
