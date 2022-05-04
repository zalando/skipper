package kubernetes

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/zalando/skipper/dataclients/kubernetes/definitions"
	"github.com/zalando/skipper/eskip"
	"github.com/zalando/skipper/filters"
	"github.com/zalando/skipper/filters/flowid"
	"github.com/zalando/skipper/loadbalancer"
	"github.com/zalando/skipper/predicates"
)

const (
	fabricAdditionalFiltersAnnotation = "fabric/additional-filters"

	defaultClusterClientRatelimitHeader = "Authorization"
	defaultUidKey                       = "https://identity.zalando.com/managed-id"
	defaultUserListPredicateName        = predicates.JWTPayloadAnyKVName
	defaultUserRealmPredicateName       = predicates.JWTPayloadAnyKVName
	defaultCheckCommonScopeFilterName   = filters.OAuthTokeninfoAllScopeName
	defaultCheckServiceFilterName       = filters.OAuthTokeninfoAnyKVName
)

var (
	defaultCheckEmployeeFilter = eskip.Filter{
		Name: filters.OAuthTokeninfoAnyKVName,
		Args: []interface{}{"realm", "/employees"},
	}
	defaultCheckServiceFilter = eskip.Filter{
		Name: filters.OAuthTokeninfoAnyKVName,
		Args: []interface{}{"realm", "/services"},
	}
	defaultCheckEmployeeOrServiceFilter = eskip.Filter{
		Name: filters.OAuthTokeninfoAnyKVName,
		Args: []interface{}{"realm", "/services", "realm", "/employees"},
	}
	defaultCheckCommonScopeFilter = eskip.Filter{
		Name: filters.OAuthTokeninfoAllScopeName,
		Args: []interface{}{"uid"},
	}
	defaultLogServiceFilter = eskip.Filter{
		Name: filters.UnverifiedAuditLogName,
		Args: []interface{}{"sub"},
	}
	defaultLogUserFilter = eskip.Filter{
		Name: filters.UnverifiedAuditLogName,
		Args: []interface{}{defaultUidKey},
	}
	defaultForwardTokenServiceFilter = eskip.Filter{
		Name: filters.ForwardTokenName,
		Args: []interface{}{
			"X-TokenInfo-Forward",
			"uid",
			"scope",
			"realm",
		},
	}
	defaultForwardTokenEmployeeFilter = eskip.Filter{
		Name: filters.ForwardTokenName,
		Args: []interface{}{
			"X-TokenInfo-Forward",
			"uid",
			"realm",
		},
	}
	// flowId("reuse")
	defaultFlowIDFilter = eskip.Filter{
		Name: filters.FlowIdName,
		Args: []interface{}{
			flowid.ReuseParameterValue,
		},
	}

	defaultUserRealmPredicateArgs = []interface{}{
		"https://identity.zalando.com/realm",
		"users",
	}
	// JWTPayloadAnyKV()
	defaultUserRealmPredicate = eskip.Predicate{
		Name: defaultUserListPredicateName,
		Args: defaultUserRealmPredicateArgs,
	}
)

type fabricGateways struct {
	clusterClientRatelimitHeader string
	uidKey                       string
	userListPredicateName        string
	userRealmPredicateName       string
	checkCommonScopeFilterName   string
	checkServiceFilterName       string

	// filters
	checkEmployeeFilter          []*eskip.Filter
	checkServiceFilter           []*eskip.Filter
	checkEmployeeOrServiceFilter []*eskip.Filter
	checkCommonScopeFilter       []*eskip.Filter
	logServiceFilter             []*eskip.Filter
	logUserFilter                []*eskip.Filter
	forwardTokenServiceFilter    []*eskip.Filter
	forwardTokenEmployeeFilter   []*eskip.Filter
	flowIDFilter                 []*eskip.Filter

	// predicates
	userRealmPredicates                 []*eskip.Predicate
	serviceUidSelectorPredicateTemplate *eskip.Predicate
}

// FabricOptions are used to customize filter, predicates and
// arguments used to identify certain conditions to your needs.
type FabricOptions struct {
	// filter + args
	CheckEmployeeFilter          string
	CheckServiceFilter           string
	CheckEmployeeOrServiceFilter string
	CheckCommonScopeFilter       string
	ForwardTokenServiceFilter    string
	ForwardTokenEmployeeFilter   string
	LogServiceFilter             string
	LogUserFilter                string
	FlowIDFilter                 string

	// ClusterClientRatelimitHeader is used for identifying a
	// client in a given request
	ClusterClientRatelimitHeader string
	// UidKey is used in x-fabric-admins and
	// x-fabric-employee-access to identify the user Claim to
	// match the value against an allow list and to identify the
	// user in the LogUserFilter.
	UidKey string
	// UserListPredicate is the predicate name used in
	// x-fabric-admins and x-fabric-employee-access to identify a
	// user in the "allow_list"
	UserListPredicate string
	// UserRealmPredicate is the predicate name used in
	// x-fabric-employee-access to identify "allow_all" or
	// "deny_all" and in x-fabric-admins if it's a user
	UserRealmPredicate string
}

type eskipBackend struct {
	Type        eskip.BackendType
	backend     string
	lbAlgorithm string
	lbEndpoints []string
}

func configureFilters(s string, defaultFilter *eskip.Filter) []*eskip.Filter {
	if s == "" {
		log.Debugf("configureFilters fallback to default %s", defaultFilter)
		return []*eskip.Filter{defaultFilter}
	}
	target, err := eskip.ParseFilters(s)
	if err != nil {
		log.Errorf("Failed to parse filter (fallback to default) string '%s': %v", s, err)
		return []*eskip.Filter{defaultFilter}
	}
	log.Infof("%d target Filters set: %s", len(target), s)
	return target
}

func (fgs *fabricGateways) convert(s *clusterState, df defaultFilters) ([]*eskip.Route, error) {
	routes := make([]*eskip.Route, 0, len(s.fabricGateways))
	for _, fg := range s.fabricGateways {
		r, err := fgs.convertOne(fg, s)
		if err != nil {
			log.Errorf("Ignore: %v", err)
			continue
		}

		routes = append(routes, r...)
	}
	return routes, nil

}

func newFabricGateways(opt Options) (*fabricGateways, error) {
	o := opt.FabricOptions
	fg := &fabricGateways{
		clusterClientRatelimitHeader: defaultClusterClientRatelimitHeader,
		uidKey:                       defaultUidKey,
		userListPredicateName:        defaultUserListPredicateName,
		userRealmPredicateName:       defaultUserRealmPredicateName,
		checkCommonScopeFilterName:   defaultCheckCommonScopeFilterName,
		checkServiceFilterName:       defaultCheckServiceFilterName,
		userRealmPredicates:          []*eskip.Predicate{&defaultUserRealmPredicate},

		// TODO(sszuecs): requires config via FabricOptions
		// this needs to be dynamic, because of it's the selector for ratelimit target set for specific apps
		serviceUidSelectorPredicateTemplate: &eskip.Predicate{
			Name: predicates.JWTPayloadAllKVName,
			Args: []interface{}{"sub"},
		},
	}
	fg.checkEmployeeFilter = configureFilters(o.CheckEmployeeFilter, &defaultCheckEmployeeFilter)
	fg.checkServiceFilter = configureFilters(o.CheckServiceFilter, &defaultCheckServiceFilter)
	fg.checkEmployeeOrServiceFilter = configureFilters(o.CheckEmployeeOrServiceFilter, &defaultCheckEmployeeOrServiceFilter)
	fg.checkCommonScopeFilter = configureFilters(o.CheckCommonScopeFilter, &defaultCheckCommonScopeFilter)
	fg.logServiceFilter = configureFilters(o.LogServiceFilter, &defaultLogServiceFilter)
	fg.logUserFilter = configureFilters(o.LogUserFilter, &defaultLogUserFilter)
	fg.forwardTokenServiceFilter = configureFilters(o.ForwardTokenServiceFilter, &defaultForwardTokenServiceFilter)
	fg.forwardTokenEmployeeFilter = configureFilters(o.ForwardTokenEmployeeFilter, &defaultForwardTokenEmployeeFilter)
	fg.flowIDFilter = configureFilters(o.FlowIDFilter, &defaultFlowIDFilter)

	if o.ClusterClientRatelimitHeader != "" {
		fg.clusterClientRatelimitHeader = o.ClusterClientRatelimitHeader
	}
	if o.UidKey != "" {
		fg.uidKey = o.UidKey
	}
	if o.UserListPredicate != "" {
		fg.userListPredicateName = o.UserListPredicate
	}
	if o.UserRealmPredicate != "" {
		var err error
		fg.userRealmPredicates, err = eskip.ParsePredicates(o.UserRealmPredicate)
		if err != nil {
			log.Errorf("Parse '%s' failed: %v", o.UserRealmPredicate, err)
			return nil, fmt.Errorf("parse '%s' failed: %w", o.UserRealmPredicate, err)
		}
	}
	log.Infof("DC: %+v", fg)
	log.Infof("filter: %d", len(fg.flowIDFilter))

	return fg, nil
}

func createRejectRouteID(fg *definitions.FabricItem, host string) string {
	return createRouteID("fg_reject", fg.Metadata.Name, fg.Metadata.Namespace, host, "", "")
}

func create404RouteID(fg *definitions.FabricItem, host string) string {
	return createRouteID("fg_404", fg.Metadata.Name, fg.Metadata.Namespace, host, "", "")
}

func createCorsRouteID(fg *definitions.FabricItem, host, path string) string {
	return createRouteID("fg_cors", fg.Metadata.Name, fg.Metadata.Namespace, host, path, "")
}

func createAdminRouteID(fg *definitions.FabricItem, host, path string) string {
	return createRouteID("fg_admin", fg.Metadata.Name, fg.Metadata.Namespace, host, path, "")
}

func createRouteID(prefix, name, namespace, host, path, method string) string {
	namespace = nonWord.ReplaceAllString(namespace, "_")
	name = nonWord.ReplaceAllString(name, "_")
	host = nonWord.ReplaceAllString(host, "_")
	path = nonWord.ReplaceAllString(path, "_")
	method = nonWord.ReplaceAllString(method, "_")

	return fmt.Sprintf("%s_%s_%s_%s_%s_%s", prefix, namespace, name, host, path, method)
}

// getKubeSvc returns serviceName, portName, portNumber, if portName is emtpy,
// portNumber will have a non zero number.
func getKubeSvc(fabsvc *definitions.FabricService) (string, string, int) {
	var (
		portName   string
		portNumber int
		err        error
	)
	portNumber, err = strconv.Atoi(fabsvc.ServicePort)
	if err != nil {
		portName = fabsvc.ServicePort
	}

	return fabsvc.ServiceName, portName, portNumber
}

// decideAllowedServices returns a definitive list of allowed services as a result of
// inspecting both global (default) and local (specified on path/method) allow list.
// Output should be interpreted as follows:
// - nil: all services are allowed
// - empty slice: no services are allowed
// - non-empty slice: a list of allowed services
func decideAllowedServices(globalAllowList []string, localAllowList *definitions.FabricAllowList) []string {
	if localAllowList != nil {
		if localAllowList.State == "disabled" {
			return nil
		} else {
			return localAllowList.UIDs
		}
	} else {
		return globalAllowList
	}
}

func allowedServicesToFilterArgs(allowedServices []string) []interface{} {
	var filterArgs []interface{}
	for _, svcName := range allowedServices {
		filterArgs = append(filterArgs, "sub", svcName)
	}

	return filterArgs
}

func applyCompression(r *eskip.Route, fc *definitions.FabricCompression) {
	if fc == nil {
		return
	}

	r.Filters = append(r.Filters, &eskip.Filter{
		Name: "compress",
		Args: []interface{}{
			fc.Factor,
			fc.Encoding,
		},
	})
}

func applyAnnotation(r *eskip.Route, m *definitions.Metadata) {
	if s, ok := m.Annotations[fabricAdditionalFiltersAnnotation]; ok {
		fs, err := eskip.ParseFilters(s)
		if err != nil {
			log.Errorf("Failed to parse filter %s/%s value '%s': %v", m.Namespace, m.Name, s, err)
			return
		}
		r.Filters = append(r.Filters, fs...)
	}
}

func (fgs *fabricGateways) convertOne(fg *definitions.FabricItem, state *clusterState) ([]*eskip.Route, error) {
	routes := make([]*eskip.Route, 0)

	lbAlgorithm := loadbalancer.RoundRobin.String()
	if s, ok := fg.Metadata.Annotations[skipperLoadBalancerAnnotationKey]; ok {
		lbAlgorithm = s
	}

	// x-fabric-admins preparation
	var adminArgs []interface{}
	if admins := fg.Spec.Admins; len(admins) != 0 {
		adminArgs = make([]interface{}, 0, 2*len(admins))
		for _, s := range admins {
			adminArgs = append(adminArgs, defaultUidKey, s)
		}
	}

	// x-fabric-cors-support preparation
	var allowedOrigins []interface{}
	if fg.Spec.Cors != nil {
		cors := fg.Spec.Cors
		allowedOrigins = make([]interface{}, 0, len(cors.AllowedOrigins))
		sort.Strings(cors.AllowedOrigins)
		for _, w := range cors.AllowedOrigins {
			// explicitly disallow * by design
			if w != "*" {
				allowedOrigins = append(allowedOrigins, "https://"+w)
			}
		}
	}

	// x-external-service-provider
	if esp := fg.Spec.ExternalServiceProvider; esp != nil {
		trs, err := state.getStacksetTraffic(fg.Metadata.Namespace, fg.Metadata.Name)
		if err != nil {
			return nil, fmt.Errorf("no traffic for x-external-service-provider: %w", err)
		}

		weightsMap, noopCount := calculateTrafficForStackset(trs)

		globalHostRouteDone := false
		for i, traffic := range trs {
			if traffic.Weight <= 0 {
				continue
			}

			var trafficParam float64
			if v, ok := weightsMap[traffic.ServiceName]; ok {
				trafficParam = v
			} else {
				continue
			}

			ridSuffix := ""
			if i >= 0 {
				ridSuffix = "_" + strconv.Itoa(i)
			}
			println("trafficParam:", trafficParam, "noopCount:", noopCount, "ridSuffix:", ridSuffix)

			endpoints := state.getEndpointsByService(fg.Metadata.Namespace, traffic.ServiceName, "http", &servicePort{
				Name: traffic.ServicePort.StrVal,
				Port: traffic.ServicePort.IntValue(),
			})
			// TODO(sszuecs): maybe check that endpoints are not 0, but what if all of them are 0. Maybe better to shortcut the routes with `status(502) -> <shunt>` in this case.

			for _, host := range esp.Hosts {
				log.Debugf("x-external-service-provider host=%s svc=%s portName=%s, portNumber=%d", host, traffic.ServiceName, traffic.ServicePort.StrVal, traffic.ServicePort.IntValue())

				routes = append(routes, fgs.createRoutes(fg, globalHostRouteDone, trafficParam, noopCount, ridSuffix, host, lbAlgorithm, endpoints, adminArgs, allowedOrigins)...)
			}
			globalHostRouteDone = true
			noopCount--
		}

	}

	// x-fabric-service
	for _, fabsvc := range fg.Spec.Service {
		host := fabsvc.Host
		svc, err := state.getService(fg.Metadata.Namespace, fabsvc.ServiceName)
		if err != nil {
			return nil, fmt.Errorf("failed to find service for %s/%s x-fabric-service: %w", fg.Metadata.Namespace, fabsvc.ServiceName, err)
		}

		endpoints := state.getEndpointsByService(svc.Meta.Namespace, svc.Meta.Name, "http", &servicePort{
			Name:       svc.Spec.Ports[0].Name,
			Port:       svc.Spec.Ports[0].Port,
			TargetPort: svc.Spec.Ports[0].TargetPort,
		})
		log.Debugf("fabsvc host=%s svc=%s portName=%s, portNumber=%d", host, svc.Meta.Name,
			svc.Spec.Ports[0].Name,
			svc.Spec.Ports[0].Port)

		routes = append(routes, fgs.createRoutes(fg, false, -1, -1, "", host, lbAlgorithm, endpoints, adminArgs, allowedOrigins)...)

	}

	// TODO(sszuecs): make sure errors are reported
	// Do we have errors here that we need to report and skip routes creation?

	// TODO(sszuecs): clean this up
	fd, err := os.Create("/tmp/foo.eskip")
	if err != nil {
		return nil, fmt.Errorf("failed to convert fabricgateway %s/%s: %w", fg.Metadata.Namespace, fg.Metadata.Name, err)
	}
	defer fd.Close()
	eskip.Fprint(fd, eskip.PrettyPrintInfo{Pretty: true, IndentStr: "\t"}, routes...)

	return routes, nil
}

// calculateTrafficForStackset returns parameters that can be feed into Traffic()
// predicates and the max count of True() predicates to be used and to
// be decreased by the user of this function.
func calculateTrafficForStackset(trs []*definitions.ActualTraffic) (map[string]float64, int) {
	trafficMap := make(map[string]float64)
	noopCount := 0
	var weightsSum float64
	for _, traffic := range trs {
		if traffic.Weight <= 0 {
			continue
		}
		trafficMap[traffic.ServiceName] = traffic.Weight
		weightsSum += traffic.Weight
		noopCount += 1
	}
	noopCount -= 2 // 1 route has no Traffic(), 1 route has only Traffic(), and rest needs True()s
	if noopCount < 0 {
		noopCount = 0
	}

	keys := make([]string, 0, len(trafficMap))
	for k := range trafficMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// used to pass to createRoutes() as the arg in Traffic(arg).
	restIterations := len(trafficMap) - 1
	weightsMap := make(map[string]float64)
	for _, k := range keys {
		if restIterations == 0 {
			weightsMap[k] = float64(-1) // last has no Traffic() or we loose traffic
			break
		}
		v := trafficMap[k]
		weightsMap[k] = v / weightsSum
		weightsSum -= v
		restIterations -= 1
	}

	return weightsMap, noopCount
}

func (fgs *fabricGateways) createRoutes(fg *definitions.FabricItem, hostGlobalRouteDone bool, trafficParam float64, noopCount int, ridSuffix, host, lbAlgorithm string, endpoints []string, adminArgs, allowedOrigins []interface{}) []*eskip.Route {
	routes := make([]*eskip.Route, 0)

	be := ""
	var bt eskip.BackendType = eskip.LBBackend
	if len(endpoints) == 1 {
		be = endpoints[0]
		bt = eskip.NetworkBackend
		endpoints = nil
	}
	eskipBackend := &eskipBackend{
		Type:        bt,
		backend:     be,
		lbAlgorithm: lbAlgorithm,
		lbEndpoints: endpoints,
	}

	defaultScopePrivileges := []interface{}{
		"uid",
	}

	if !hostGlobalRouteDone {
		// 404 route per host
		r404 := fgs.create404Route(create404RouteID(fg, host), host, defaultScopePrivileges)
		routes = append(routes, r404)

		// reject plain http per host with 400, but not for internal routes
		if !strings.HasSuffix(host, ".cluster.local") {
			reject400 := fgs.createRejectRoute(createRejectRouteID(fg, host), host, defaultScopePrivileges)
			routes = append(routes, reject400)
		}
	}

	for _, fp := range fg.Spec.Paths.Path {
		methods := make([]string, 0, len(fp.Methods))
		for _, m := range fp.Methods {
			methods = append(methods, m.Method)

			// AllowList per method and global default
			//     example: oauthTokeninfoAllScope("uid", "foo.write")
			var privs []interface{}
			privs = append(privs, defaultScopePrivileges...)
			for _, priv := range m.Privileges {
				privs = append(privs, priv)
			}

			allowedServices := decideAllowedServices(fg.Spec.AllowList, m.AllowList)
			if len(allowedServices) > 0 || allowedServices == nil {
				r := fgs.createServiceRoute(m, eskipBackend, allowedOrigins, allowedServicesToFilterArgs(allowedServices), privs, fg.Metadata.Name, fg.Metadata.Namespace, host, fp.Path, ridSuffix)
				applyPath(r, fp)
				applyCompression(r, fg.Spec.Compression)
				applyStaticResponse(r, m.Response)
				applyTraffic(r, trafficParam)
				applyNoops(r, noopCount)
				applyAnnotation(r, fg.Metadata)
				routes = append(routes, r)

				// ratelimit overrrides require separated routes with predicates.JWTPayloadAllKVName
				if m.Ratelimit != nil && len(m.Ratelimit.Target) > 0 {
					routes = append(routes, fgs.createRatelimitRoutes(r, m, fg.Metadata.Name, fp.Path)...)
				}
			}

			// routes to support x-fabric-employee-access
			if m.EmployeeAccess != nil {
				usersAllowed := make([]interface{}, 0, 2*len(m.EmployeeAccess.UserList))
				sort.Strings(m.EmployeeAccess.UserList)
				for _, u := range m.EmployeeAccess.UserList {
					usersAllowed = append(usersAllowed, defaultUidKey, u)
				}
				rea := fgs.createEmployeeAccessRoute(m, eskipBackend, allowedOrigins, usersAllowed, m.EmployeeAccess.Type, fg.Metadata.Name, fg.Metadata.Namespace, host, fp.Path, ridSuffix)
				applyPath(rea, fp)
				applyCompression(rea, fg.Spec.Compression)
				applyStaticResponse(rea, m.Response)
				applyTraffic(rea, trafficParam)
				applyNoops(rea, noopCount)
				routes = append(routes, rea)
			}

			// routes to support x-fabric-admins
			if len(adminArgs) != 0 {
				// TODO(sszuecs): currently fabric would also do applyStaticResponse in case we have it for the route, let's discuss if it makes sense. https://github.com/zalando-incubator/fabric-gateway/pull/64 says it does make sense, because admins want to try that the static response is in place.
				ra := fgs.createAdminRoute(eskipBackend, createAdminRouteID(fg, host, fp.Path)+ridSuffix, host, fp.Path, m.Method, adminArgs, allowedOrigins)
				applyPath(ra, fp)
				applyCompression(ra, fg.Spec.Compression)
				applyStaticResponse(ra, m.Response)
				applyTraffic(ra, trafficParam)
				applyNoops(ra, noopCount)
				routes = append(routes, ra)
			}

		}

		if !hostGlobalRouteDone && fg.Spec.Cors != nil && len(allowedOrigins) > 0 {
			rID := createCorsRouteID(fg, host, fp.Path)
			corsMethods := strings.ToUpper(strings.Join(methods, ", "))
			if !strings.Contains(corsMethods, "OPTIONS") {
				corsMethods = corsMethods + ", OPTIONS"
			}
			corsAllowedHeaders := strings.Join(fg.Spec.Cors.AllowedHeaders, ", ")
			cr := fgs.createCorsRoute(rID, host, fp.Path, corsMethods, corsAllowedHeaders, methods, allowedOrigins)
			applyPath(cr, fp)
			applyTraffic(cr, trafficParam)
			applyNoops(cr, noopCount)
			routes = append(routes, cr)
		}
	}
	return routes
}

func (fgs *fabricGateways) create404Route(rid, host string, privs []interface{}) *eskip.Route {
	r := &eskip.Route{
		Id: rid,
		Predicates: []*eskip.Predicate{
			{
				Name: predicates.PathSubtreeName,
				Args: []interface{}{
					"/",
				},
			}, {
				Name: predicates.HostAnyName,
				Args: []interface{}{
					host,
					host + ":80",
					host + ":443",
				},
			},
		},
		Filters:     append(fgs.checkCommonScopeFilter, fgs.logServiceFilter...),
		BackendType: eskip.ShuntBackend,
	}

	r.Filters = append(r.Filters,
		&eskip.Filter{
			Name: filters.StatusName,
			Args: []interface{}{
				404,
			},
		},
		&eskip.Filter{
			Name: filters.InlineContentName,
			Args: []interface{}{
				`{"title":"Gateway Rejected","status":404,"detail":"Gateway Route Not Matched"}`,
			},
		},
	)

	return r
}

func (fgs *fabricGateways) createRejectRoute(rid, host string, privs []interface{}) *eskip.Route {
	r := &eskip.Route{
		Id: rid,
		Predicates: []*eskip.Predicate{
			{
				Name: predicates.PathSubtreeName,
				Args: []interface{}{
					"/",
				},
			}, {
				Name: predicates.HostAnyName,
				Args: []interface{}{
					host,
					host + ":443",
				},
			}, {
				Name: predicates.HeaderName,
				Args: []interface{}{
					"X-Forwarded-Proto",
					"http",
				},
			},
		},
		Filters:     append(fgs.checkCommonScopeFilter, fgs.logServiceFilter...),
		BackendType: eskip.ShuntBackend,
	}

	r.Filters = append(r.Filters,
		&eskip.Filter{
			Name: filters.StatusName,
			Args: []interface{}{
				400,
			},
		},
		&eskip.Filter{
			Name: filters.InlineContentName,
			Args: []interface{}{
				`{"title":"Gateway Rejected","status":400,"detail":"TLS is required","type":"https://cloud.docs.zalando.net/howtos/ingress/#redirect-http-to-https"}`,
			},
		},
	)

	return r
}

func (fgs *fabricGateways) createEmployeeAccessRoute(m *definitions.FabricMethod, eskipBackend *eskipBackend, allowedOrigins, userList []interface{}, accessType, name, namespace, host, path, ridSuffix string) *eskip.Route {
	r := &eskip.Route{
		Id:     createRouteID("fg_eaccess", name, namespace, host, path, m.Method) + ridSuffix,
		Method: strings.ToUpper(m.Method),
		Predicates: []*eskip.Predicate{
			{
				Name: predicates.WeightName,
				Args: []interface{}{
					4, // TODO(sszuecs) needs checking
				},
			},
		},
		Filters:     append(fgs.checkEmployeeFilter, fgs.checkCommonScopeFilter...),
		BackendType: eskipBackend.Type,
		Backend:     eskipBackend.backend,
		LBAlgorithm: eskipBackend.lbAlgorithm,
		LBEndpoints: eskipBackend.lbEndpoints,
	}
	r.Filters = append(r.Filters, fgs.logServiceFilter...)

	// add optional ratelimit only default ratelimit
	if m.Ratelimit != nil {
		r.Filters = append(r.Filters,
			&eskip.Filter{
				//inlineContentIfStatus(429, "{\"title\": \"Rate limit exceeded\", \"detail\": \"See the retry-after header for how many seconds to wait before retrying.\", \"status\": 429}", "application/problem+json")
				Name: filters.InlineContentIfStatusName,
				Args: []interface{}{
					429,
					"{\"title\":\"Rate limit exceeded\",\"detail\":\"See the retry-after header for how many seconds to wait before retrying.\",\"status\":429}",
					"application/problem+json",
				},
			},
			&eskip.Filter{
				// clusterClientRatelimit("foo_.._users", 30, "1m", "Authorization")
				Name: filters.ClusterClientRatelimitName,
				Args: []interface{}{
					// TODO(sszuecs): maybe we want to add namespace here, too (assume people could use namespaces to separate prod/staging, this would otherwise count both)
					fmt.Sprintf("%s_%s_%s",
						name,
						strings.Trim(nonWord.ReplaceAllString(path, "-"), "-"),
						m.Method,
					),
					m.Ratelimit.DefaultRate,
					m.Ratelimit.Period,
					fgs.clusterClientRatelimitHeader,
				},
			},
		)
	}

	// add rest filters
	r.Filters = append(r.Filters, fgs.flowIDFilter...)
	r.Filters = append(r.Filters, fgs.forwardTokenEmployeeFilter...)

	// optional cors
	if len(allowedOrigins) > 0 {
		r.Filters = append(r.Filters,
			&eskip.Filter{
				// corsOrigin("https://foo.example.org", "https://bar.example.com")
				Name: filters.CorsOriginName,
				Args: allowedOrigins,
			},
		)
	}

	applyCommonPredicates(r, host)

	// x-fabric-employee-access specifics
	switch accessType {
	case "allow_all":
		// allow all
		r.Predicates = append(r.Predicates, fgs.userRealmPredicates...)
	case "allow_list":
		r.Predicates = append(r.Predicates, &eskip.Predicate{
			Name: fgs.userListPredicateName,
			Args: userList,
		})
	case "deny_all":
		r.Predicates = append(r.Predicates, fgs.userRealmPredicates...)
		// no need to process filters, reset filters and set backend to shunt
		r.Filters = []*eskip.Filter{
			{
				Name: filters.StatusName,
				Args: []interface{}{403}, // TODO(sszuecs): status similar to fg-controller?
			},
			{
				// TODO(sszuecs): what would the current FG-controller do to return a response message?
				Name: filters.InlineContentName,
				Args: []interface{}{
					`{"title":"Gateway Rejected","status":403,"detail":"deny all employees"}`,
				},
			},
		}
		r.BackendType = eskip.ShuntBackend
		r.Backend = ""
		r.LBAlgorithm = ""
		r.LBEndpoints = nil
	}

	return r
}

func applyStaticResponse(r *eskip.Route, static *definitions.FabricResponse) {
	if static != nil {
		r.BackendType = eskip.ShuntBackend
		r.Backend = ""
		r.LBAlgorithm = ""
		r.LBEndpoints = nil

		headers := make([]interface{}, 0, 2*len(static.Headers))
		for _, k := range getSortedKeysStr(static.Headers) {
			headers = append(headers, k)
			headers = append(headers, static.Headers[k])
		}

		r.Filters = append(r.Filters,
			// -> setResponseHeader("Content-Type", "application/problem+json")
			&eskip.Filter{
				Name: filters.SetResponseHeaderName,
				Args: headers,
			},
			// -> status(501)
			&eskip.Filter{
				Name: filters.StatusName,
				Args: []interface{}{static.StatusCode},
			},
			// -> inlineContent("{\"title\": \"Issues Updates Not Yet Supported\", \"status\": 501}")
			&eskip.Filter{
				Name: filters.InlineContentName,
				Args: []interface{}{
					static.Body,
				},
			},
		)
	}
}

func applyNoops(r *eskip.Route, noopCount int) {
	if noopCount < 1 {
		return
	}
	for i := 0; i < noopCount; i++ {
		r.Predicates = append(r.Predicates, &eskip.Predicate{
			Name: predicates.TrueName,
		})
	}
}

func applyTraffic(r *eskip.Route, trafficParam float64) {
	if trafficParam < 0 || trafficParam > 1 {
		return
	}
	r.Predicates = append(r.Predicates, &eskip.Predicate{
		Name: predicates.TrafficName,
		Args: []interface{}{
			trafficParam,
		},
	})
}

func applyCommonPredicates(r *eskip.Route, host string) {
	if !strings.HasSuffix(host, ".cluster.local") {
		r.Predicates = append(
			[]*eskip.Predicate{
				{
					Name: predicates.HostAnyName,
					Args: []interface{}{
						host,
						host + ":443",
					},
				},
				{
					Name: predicates.HeaderName,
					Args: []interface{}{
						"X-Forwarded-Proto",
						"https",
					},
				},
			}, r.Predicates...)
	} else {
		r.Predicates = append(
			[]*eskip.Predicate{
				{
					Name: predicates.HostAnyName,
					Args: []interface{}{
						host,
						host + ":80",
					},
				},
			}, r.Predicates...)
	}
}

func (fgs *fabricGateways) createServiceRoute(m *definitions.FabricMethod, eskipBackend *eskipBackend, allowedOrigins, allowedServices, privs []interface{}, name, namespace, host, path, ridSuffix string) *eskip.Route {
	r := &eskip.Route{
		Id:     createRouteID("fg", name, namespace, host, path, m.Method) + ridSuffix,
		Method: strings.ToUpper(m.Method),
		Predicates: []*eskip.Predicate{
			{
				Name: predicates.WeightName,
				Args: []interface{}{
					23, // TODO(sszuecs) needs checking
				},
			},
		},
		Filters: append(fgs.checkEmployeeOrServiceFilter, &eskip.Filter{
			// oauthTokeninfoAllScope("uid", "foo.write")
			Name: fgs.checkCommonScopeFilterName,
			Args: privs,
		}),
		BackendType: eskipBackend.Type,
		Backend:     eskipBackend.backend,
		LBAlgorithm: eskipBackend.lbAlgorithm,
		LBEndpoints: eskipBackend.lbEndpoints,
	}

	// allow list via x-fabric-whitelist configuration
	if len(allowedServices) > 0 {
		// oauthTokeninfoAnyKV("sub", "my-app1", "sub", "my-app2")
		r.Filters = append(r.Filters,
			&eskip.Filter{
				Name: fgs.checkServiceFilterName,
				Args: allowedServices,
			},
		)
	}

	r.Filters = append(r.Filters, fgs.logServiceFilter...)

	// add optional ratelimit (default ratelimit here, overrides later below adding new routes)
	if m.Ratelimit != nil {
		r.Filters = append(r.Filters,
			&eskip.Filter{
				//inlineContentIfStatus(429, "{\"title\": \"Rate limit exceeded\", \"detail\": \"See the retry-after header for how many seconds to wait before retrying.\", \"status\": 429}", "application/problem+json")
				Name: filters.InlineContentIfStatusName,
				Args: []interface{}{
					429,
					"{\"title\":\"Rate limit exceeded\",\"detail\":\"See the retry-after header for how many seconds to wait before retrying.\",\"status\":429}",
					"application/problem+json",
				},
			},
			&eskip.Filter{
				// clusterClientRatelimit("spp-brand-service_api-brand-assignments-id_DELETE", 30, "1m", "Authorization")
				Name: filters.ClusterClientRatelimitName,
				Args: []interface{}{
					// TODO(sszuecs): maybe we want to add namespace here, too (assume people could use namespaces to separate prod/staging, this would otherwise count both)
					fmt.Sprintf("%s_%s_%s",
						name,
						strings.Trim(nonWord.ReplaceAllString(path, "-"), "-"),
						m.Method,
					),
					m.Ratelimit.DefaultRate,
					m.Ratelimit.Period,
					fgs.clusterClientRatelimitHeader,
				},
			},
		)
	}

	// add rest filters
	r.Filters = append(r.Filters, fgs.flowIDFilter...)
	r.Filters = append(r.Filters, fgs.forwardTokenServiceFilter...)

	// optional cors
	if len(allowedOrigins) > 0 {
		r.Filters = append(r.Filters,
			&eskip.Filter{
				// corsOrigin("https://foo.example.org", "https://bar.example.com")
				Name: filters.CorsOriginName,
				Args: allowedOrigins,
			},
		)
	}

	applyCommonPredicates(r, host)
	return r
}

func (fgs *fabricGateways) createRatelimitRoutes(r *eskip.Route, m *definitions.FabricMethod, name, path string) []*eskip.Route {
	routes := make([]*eskip.Route, 0, len(m.Ratelimit.Target))

	for i, rTarget := range m.Ratelimit.Target {
		rr := eskip.Copy(r)
		rr.Id = fmt.Sprintf("%s%d", rr.Id, i)

		// add predicate to match client application
		p := *fgs.serviceUidSelectorPredicateTemplate
		p.Args = append(p.Args, rTarget.UID)
		rr.Predicates = append(rr.Predicates, &p)

		// find and replace ratelimit: type, group, rate. period stays the same
		for j := range rr.Filters {
			if rr.Filters[j].Name == filters.ClusterClientRatelimitName {
				// replace clusterClientRatelimit with clusterRatelimit,
				// because we have separate routes per UID and we can scale
				// shards with clusterRatelimit
				rr.Filters[j].Name = filters.ClusterRatelimitName
				rr.Filters[j].Args = []interface{}{
					fmt.Sprintf("%s_%s_%s_%s",
						name,
						strings.Trim(nonWord.ReplaceAllString(path, "-"), "-"),
						m.Method,
						rTarget.UID,
					),
					rTarget.Rate,
					m.Ratelimit.Period,
				}
			}
		}
		routes = append(routes, rr)
	}
	return routes

}

func (fgs *fabricGateways) createAdminRoute(eskipBackend *eskipBackend, routeID, host, path, method string, adminsArgs, allowedOrigins []interface{}) *eskip.Route {
	rr := &eskip.Route{
		Id:          routeID + "_" + strings.ToLower(method),
		BackendType: eskipBackend.Type,
		Backend:     eskipBackend.backend, // in case we have only 1 endpoint we fallback to network backend
		LBAlgorithm: eskipBackend.lbAlgorithm,
		LBEndpoints: eskipBackend.lbEndpoints,
		Method:      strings.ToUpper(method),
		Predicates: []*eskip.Predicate{
			{
				Name: predicates.WeightName,
				Args: []interface{}{5},
			},
		},
		Filters: append(fgs.checkEmployeeOrServiceFilter,
			&eskip.Filter{
				// enableAccessLog(2, 4, 5)
				Name: filters.EnableAccessLogName,
				Args: []interface{}{2, 4, 5},
			}),
	}

	rr.Filters = append(rr.Filters, fgs.checkCommonScopeFilter...)
	rr.Filters = append(rr.Filters, fgs.logUserFilter...)
	rr.Filters = append(rr.Filters, fgs.flowIDFilter...)
	rr.Filters = append(rr.Filters, fgs.forwardTokenServiceFilter...)

	rr.Predicates = append(rr.Predicates, fgs.userRealmPredicates...)
	rr.Predicates = append(rr.Predicates, &eskip.Predicate{
		Name: fgs.userListPredicateName,
		Args: adminsArgs,
	})

	if len(allowedOrigins) > 0 {
		rr.Filters = append(rr.Filters, &eskip.Filter{
			// corsOrigin("https://example.org", "https://example.com")
			Name: filters.CorsOriginName,
			Args: allowedOrigins,
		})
	}

	applyCommonPredicates(rr, host)

	return rr
}

func (fgs *fabricGateways) createCorsRoute(routeID, host, path, corsMethods, corsAllowedHeaders string, methods []string, allowedOrigins []interface{}) *eskip.Route {
	r := &eskip.Route{
		Id:          routeID,
		BackendType: eskip.ShuntBackend,
		Method:      "OPTIONS",
		Predicates: []*eskip.Predicate{
			{
				Name: predicates.WeightName,
				Args: []interface{}{3},
			},
		},
		Filters: []*eskip.Filter{
			{
				//status(204)
				Name: filters.StatusName,
				Args: []interface{}{204},
			},
		},
	}
	r.Filters = append(r.Filters, fgs.flowIDFilter...)
	r.Filters = append(r.Filters,
		&eskip.Filter{ // corsOrigin("https://example.org", "https://example.com")
			Name: filters.CorsOriginName,
			Args: allowedOrigins,
		},
		&eskip.Filter{
			// appendResponseHeader("Access-Control-Allow-Methods", "DELETE, GET, OPTIONS")
			Name: filters.AppendResponseHeaderName,
			Args: stringToEmptyInterface([]string{"Access-Control-Allow-Methods", corsMethods}),
		},
		&eskip.Filter{
			// appendResponseHeader("Access-Control-Allow-Headers", "authorization, ot-tracer-sampled, ot-tracer-spanid, ot-tracer-traceid")
			Name: filters.AppendResponseHeaderName,
			Args: stringToEmptyInterface([]string{"Access-Control-Allow-Headers", corsAllowedHeaders}),
		},
	)

	applyCommonPredicates(r, host)
	return r
}
