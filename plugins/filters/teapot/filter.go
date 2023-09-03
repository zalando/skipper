package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/zalando/skipper/filters"
	"golang.org/x/text/language"
)

var _ filters.Filter = (*teapotFilter)(nil)

type teapotFilter struct {
	NextLoad     time.Time
	Services     []teapotService
	ServicesHash string
	Teapots      []teapotConfig
	TeapotsHash  string
}

func (f *teapotFilter) loadServices() {
	data, md5result, fetchErr := fetchS3File(
		os.Getenv("TEAPOT_S3_BUCKET"),
		os.Getenv("TEAPOT_S3_SERVICES_KEY"),
	)
	if fetchErr != nil {
		slog.Error("Error fetching services.json", "error", fetchErr)
		return
	}

	// Only import if the hash is different
	if f.ServicesHash == md5result {
		return
	}

	if unmarshalErr := json.Unmarshal(data, &f.Services); unmarshalErr != nil {
		slog.Error("Error reading services.json", "error", unmarshalErr)
		return
	}
	f.ServicesHash = md5result
}

func (f *teapotFilter) loadTeapots() {
	data, md5result, fetchErr := fetchS3File(
		os.Getenv("TEAPOT_S3_BUCKET"),
		os.Getenv("TEAPOT_S3_TEAPOTS_KEY"),
	)
	if fetchErr != nil {
		slog.Error("Error fetching teapots.json", "error", fetchErr)
		return
	}

	// Only import if the hash is different
	if f.TeapotsHash == md5result {
		return
	}

	if unmarshalErr := json.Unmarshal(data, &f.Teapots); unmarshalErr != nil {
		slog.Error("Error reading teapots.json", "error", unmarshalErr)
		return
	}
	f.TeapotsHash = md5result
}

func (f *teapotFilter) determineCountry(ctx filters.FilterContext) string {
	// If behind Cloudfront CDN
	cloudfrontCountry := strings.TrimSpace(ctx.Request().Header.Get("CloudFront-Viewer-Country"))
	if cloudfrontCountry != "" {
		return cloudfrontCountry
	}

	// If behind Cloudflare CDN
	cloudflareCountry := strings.TrimSpace(ctx.Request().Header.Get("CF-IPCountry"))
	if cloudflareCountry != "" {
		return cloudflareCountry
	}

	return "GB" // Fallback to UK
}

func (f *teapotFilter) sendTeapotMessage(ctx filters.FilterContext, teapot teapotConfig, global bool) {
	var languageTags []language.Tag
	for lang := range teapot.Message {
		languageTags = append(languageTags, language.Make(lang))
	}

	var matcher = language.NewMatcher(languageTags)
	accept := ctx.Request().Header.Get("Accept-Language")
	tag, _ := language.MatchStrings(matcher, "", accept)
	locale := tag.String()

	if len(locale) > 5 {
		locale = strings.Split(locale, "-")[0]
	}

	var titleLanguageTags []language.Tag
	for lang := range teapot.Title {
		titleLanguageTags = append(titleLanguageTags, language.Make(lang))
	}

	matcher = language.NewMatcher(titleLanguageTags)
	tag, _ = language.MatchStrings(matcher, "", accept)
	titleLocale := tag.String()

	if len(titleLocale) > 5 {
		titleLocale = strings.Split(titleLocale, "-")[0]
	}

	ctx.Logger().Debugf("Locale: %s", locale)

	response := &teapotResponse{
		PredictedUptimeTimestampUTC: teapot.EndsAt.UTC().Format(time.RFC3339),
		Global:                      global,
	}

	message := teapot.Message[locale]
	if strings.Contains(teapot.Message[locale], "%s") {
		message = fmt.Sprintf(teapot.Message[locale], teapot.EndsAt.UTC().Format("3:04pm UTC"))
	}
	message = strings.TrimSpace(message)
	if len(message) > 0 {
		response.Message = &message
	}

	title := teapot.Title[titleLocale]
	title = strings.TrimSpace(title)
	if len(title) > 0 {
		response.Title = &title
	}

	jsonResponse, _ := json.Marshal(
		&teapotError{
			Status: http.StatusTeapot,
			Error:  *response,
		},
	)

	header := http.Header{}
	header.Set("Content-Type", "application/json")

	ctx.Serve(
		&http.Response{
			StatusCode: http.StatusTeapot,
			Header:     header,
			Body:       io.NopCloser(bytes.NewBufferString(string(jsonResponse))),
		},
	)
}

func (f *teapotFilter) Request(ctx filters.FilterContext) {
	if time.Now().After(f.NextLoad) {
		ctx.Logger().Debugf(
			"Teapot Reload required",
		)

		f.NextLoad = time.Now().Add(30 * time.Second)
		go f.loadServices()
		go f.loadTeapots()
	}

	ctx.Logger().Debugf("Teapot Route: %q. Next Load: %q", ctx.Request().RequestURI, f.NextLoad)

	ipAddress := strings.TrimSpace(ctx.Request().Header.Get("Cf-Connecting-Ip"))
	for whitelistIP, name := range map[string]string{
		"151.224.191.144": "David",
		"2.100.105.116":   "Mark",
		"188.127.93.222":  "Office",
	} {
		if whitelistIP == ipAddress {
			ctx.Logger().Infof("IP address %q has been whitelisted for %q", whitelistIP, name)
			return
		}
	}

	ctx.Logger().Infof("IP address %q is not whitelisted\n", ipAddress)

	// Get the country code
	visitorCountryCode := f.determineCountry(ctx)

	// Check for teapot
	for _, teapot := range f.Teapots {
		if teapot.Enabled {
			// Check if we have gone over the estimated time
			if teapot.EndsAt.Before(time.Now().UTC()) {
				teapot.EndsAt = time.
					Now().
					Round(time.Duration(teapot.ExtendBy) * time.Minute).
					Add(time.Duration(teapot.ExtendBy) * time.Minute)
			}

			// Check if we have ignored this country
			ignored := false
			for _, country := range teapot.IgnoreCountries {
				if country == visitorCountryCode {
					ignored = true
					break
				}
			}

			if ignored {
				// We are ignoring this country - continue to next teapot
				continue
			}

			// Check if we are only teapotting for this country
			if len(teapot.OnlyCountries) > 0 {
				found := false
				for _, country := range teapot.OnlyCountries {
					if country == visitorCountryCode {
						found = true
						break
					}
				}

				if !found {
					// This country isn't in the list - continue to next teapot
					continue
				}
			}

			ctx.Logger().Infof("Teapot enabled")

			for _, TService := range teapot.Services {
				for _, service := range f.Services {
					if service.Name == TService {
						ctx.Logger().Infof(">>>>>> %s\n", service.Name)
						for _, route := range service.Routes {
							if route.IsRegex {
								match, _ := regexp.MatchString(route.URI, ctx.Request().RequestURI)
								if match {
									ctx.Logger().Infof("Matched route %s\n", route.URI)

									f.sendTeapotMessage(ctx, teapot, TService == "all")
									return
								}
							} else {
								if strings.HasSuffix(route.URI, "*") {
									if strings.HasPrefix(ctx.Request().RequestURI, route.URI[0:len(route.URI)-1]) {
										ctx.Logger().Infof("Matched route %s\n", route.URI)
										f.sendTeapotMessage(ctx, teapot, TService == "all")
										return
									}
								} else if ctx.Request().RequestURI == route.URI {
									ctx.Logger().Infof("Matched route %s\n", route.URI)
									f.sendTeapotMessage(ctx, teapot, TService == "all")
									return
								}
							}
						}
					}
				}
			}
		}
	}
}

func (f *teapotFilter) Response(_ filters.FilterContext) {}
