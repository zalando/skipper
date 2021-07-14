package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/opentracing/opentracing-go"
	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/filters"
)

const (
	JwtValidationName = "jwtValidation"
)

type (
	jwtValidationSpec struct {
		options TokenintrospectionOptions
	}

	jwtValidationFilter struct {
		authClient      *authClient
		claims          []string
		upstreamHeaders map[string]string
	}
)

var rsakeys map[string]*rsa.PublicKey

func NewJwtValidationWithOptions(o TokenintrospectionOptions) filters.Spec {
	return &jwtValidationSpec{
		options: o,
	}
}

func NewJwtValidation(timeout time.Duration) filters.Spec {
	return NewJwtValidationWithOptions(TokenintrospectionOptions{
		Timeout: timeout,
		Tracer:  opentracing.NoopTracer{},
	})
}

func JwtValidationWithOptions(
	create func(time.Duration) filters.Spec,
	o TokenintrospectionOptions,
) filters.Spec {
	s := create(o.Timeout)
	ts, ok := s.(*tokenIntrospectionSpec)
	if !ok {
		return s
	}

	ts.options = o
	return ts
}

func (s *jwtValidationSpec) Name() string {
	return JwtValidationName
}

func (s *jwtValidationSpec) CreateFilter(args []interface{}) (filters.Filter, error) {
	sargs, err := getStrings(args)
	if err != nil {
		return nil, err
	}
	if len(sargs) < 2 {
		return nil, filters.ErrInvalidFilterParameters
	}

	issuerURL := sargs[0]

	cfg, err := getOpenIDConfig(issuerURL)
	if err != nil {
		return nil, err
	}

	var ac *authClient
	var ok bool
	if ac, ok = issuerAuthClient[issuerURL]; !ok {
		ac, err = newAuthClient(cfg.JwksURI, tokenInfoSpanName, s.options.Timeout, s.options.MaxIdleConns, s.options.Tracer)
		if err != nil {
			return nil, filters.ErrInvalidFilterParameters
		}
		issuerAuthClient[issuerURL] = ac
	}

	f := &jwtValidationFilter{
		authClient: ac,
	}
	f.claims = strings.Split(sargs[1], " ")
	if !all(f.claims, cfg.ClaimsSupported) {
		return nil, fmt.Errorf("%v: %s, supported Claims: %v", errUnsupportedClaimSpecified, strings.Join(f.claims, ","), cfg.ClaimsSupported)
	}

	// inject additional headers from the access token for upstream applications
	if len(sargs) > 2 && sargs[2] != "" {
		f.upstreamHeaders = make(map[string]string)

		for _, header := range strings.Split(sargs[2], " ") {
			sl := strings.SplitN(header, ":", 2)
			if len(sl) != 2 || sl[0] == "" || sl[1] == "" {
				return nil, fmt.Errorf("%w: malformatted filter for upstream headers %s", filters.ErrInvalidFilterParameters, sl)
			}
			f.upstreamHeaders[sl[0]] = sl[1]
		}
		log.Debugf("Upstream Headers: %v", f.upstreamHeaders)
	}

	return f, nil
}

// String prints nicely the jwtValidationFilter configuration based on the
// configuration and check used.
func (f *jwtValidationFilter) String() string {
	return fmt.Sprintf("%s(%s)", JwtValidationName, strings.Join(f.claims, ","))
}

func (f *jwtValidationFilter) validateAnyClaims(token jwt.Token) bool {
	for _, wantedClaim := range f.claims {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if _, ok2 := claims[wantedClaim]; ok2 {
				return true
			}
		}
	}
	return false
}

func (f *jwtValidationFilter) Request(ctx filters.FilterContext) {
	r := ctx.Request()

	var info jwt.Token
	infoTemp, ok := ctx.StateBag()[oidcClaimsCacheKey]
	if !ok {
		token, ok := getToken(r)
		if !ok || token == "" {
			unauthorized(ctx, "", missingToken, f.authClient.url.Hostname(), "")
			return
		}

		body, err := f.authClient.getTokeninfo("", ctx)
		if err != nil {
			log.Errorf("Error while getting jwt keys: %v.", err)

			unauthorized(ctx, "", "jwt public keys", f.authClient.url.Hostname(), "")
			return
		}
		//var body map[string]interface{}
		//json.NewDecoder(resp.Body).Decode(&body)
		rsakeys = make(map[string]*rsa.PublicKey)
		if body["keys"] != nil {
			for _, bodykey := range body["keys"].([]interface{}) {
				key := bodykey.(map[string]interface{})
				kid := key["kid"].(string)
				rsakey := new(rsa.PublicKey)
				number, _ := base64.RawURLEncoding.DecodeString(key["n"].(string))
				rsakey.N = new(big.Int).SetBytes(number)
				rsakey.E = 65537
				rsakeys[kid] = rsakey
			}
		} else {
			log.Error("Not able to get public keys")
			unauthorized(ctx, "", "Not able to get public keys", f.authClient.url.Hostname(), "")
			return
		}

		parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
			return rsakeys[token.Header["kid"].(string)], nil
		})
		if err != nil {
			log.Errorf("Error while parsing jwt token : %v.", err)
			unauthorized(ctx, "", "error parsing jwt token", f.authClient.url.Hostname(), "")
			return
		} else if !parsedToken.Valid {
			log.Errorf("Invalid token")
			unauthorized(ctx, "", "Invalid token", f.authClient.url.Hostname(), "")
			return
		} else if parsedToken.Header["alg"] == nil {
			log.Errorf("alg must be defined")
			unauthorized(ctx, "", "alg must be defined", f.authClient.url.Hostname(), "")
			return
		}

		info = *parsedToken
		/*err = json.Unmarshal([]byte(parsedToken.Raw), &info)
		if err != nil {
			log.Errorf("Error while pasing jwt token : %v.", err)
			unauthorized(ctx, "", "error parsing jwt token", f.authClient.url.Hostname(), "")
			return*/
		//}
	} else {
		info = infoTemp.(jwt.Token)
	}

	/*sub, err := info.Sub()
	if err != nil {
		if err != errInvalidTokenintrospectionData {
			log.Errorf("Error while reading token: %v.", err)
		}

		unauthorized(ctx, sub, invalidSub, f.authClient.url.Hostname(), "")
		return
	}

	if !info.Active() {
		unauthorized(ctx, sub, inactiveToken, f.authClient.url.Hostname(), "")
		return
	}*/

	sub := info.Claims.(jwt.MapClaims)["sub"].(string)

	var allowed = f.validateAnyClaims(info)
	if !allowed {
		unauthorized(ctx, sub, invalidClaim, f.authClient.url.Hostname(), "")
		return
	}

	authorized(ctx, sub)

	var container2 tokenContainer
	container2.Claims = make(map[string]interface{})
	for claim, value := range info.Claims.(jwt.MapClaims) {
		container2.Claims[claim] = value
	}
	//container2.Claims = info.Claims.(jwt.MapClaims)

	log.Errorf("Length %d", len(container2.Claims))
	ctx.StateBag()[oidcClaimsCacheKey] = container2

	// adding upstream headers
	f.setHeaders(ctx, info.Claims.(jwt.MapClaims))
}

func (f *jwtValidationFilter) Response(filters.FilterContext) {}

// Close cleans-up the authClient
func (f *jwtValidationFilter) Close() {
	f.authClient.Close()
}

func (f *jwtValidationFilter) setHeaders(ctx filters.FilterContext, container jwt.MapClaims) (err error) {
	for key, query := range f.upstreamHeaders {
		match := container[query]
		log.Debugf("header: %s results: %s", query, match)
		if match == nil {
			log.Errorf("Lookup failed for upstream header '%s'", query)
			continue
		}
		ctx.Request().Header.Set(key, match.(string))
	}
	return
}
