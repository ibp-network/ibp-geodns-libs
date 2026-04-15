package data

import (
	"sync"
	"time"

	cfg "github.com/ibp-network/ibp-geodns-libs/config"
)

var Official = OfficialResults{
	SiteResults:     make([]SiteResult, 0),
	DomainResults:   make([]DomainResult, 0),
	EndpointResults: make([]EndpointResult, 0),
	Mu:              sync.RWMutex{},
}

type Snapshot struct {
	SiteResults     []SiteResult     `json:"site"`
	DomainResults   []DomainResult   `json:"domain"`
	EndpointResults []EndpointResult `json:"endpoint"`
}

type pendingOfficialEvent struct {
	checkType  string
	checkName  string
	memberName string
	domainName string
	endpoint   string
	status     bool
	errorText  string
	data       map[string]interface{}
	isIPv6     bool
}

func (e *pendingOfficialEvent) emit() {
	if e == nil {
		return
	}

	RecordEvent(e.checkType, e.checkName, e.memberName, e.domainName, e.endpoint, e.status, e.errorText, e.data, e.isIPv6)
}

var (
	muOfficial sync.RWMutex
	official   Snapshot
)

func GetOfficialResults() ([]SiteResult, []DomainResult, []EndpointResult) {
	muOfficial.RLock()
	defer muOfficial.RUnlock()
	return cloneSiteResults(official.SiteResults), cloneDomainResults(official.DomainResults), cloneEndpointResults(official.EndpointResults)
}

func SetOfficialSnapshot(snap Snapshot) {
	muOfficial.Lock()
	official = Snapshot{
		SiteResults:     cloneSiteResults(snap.SiteResults),
		DomainResults:   cloneDomainResults(snap.DomainResults),
		EndpointResults: cloneEndpointResults(snap.EndpointResults),
	}
	muOfficial.Unlock()
}

func BuildSnapshot(site []SiteResult, dom []DomainResult, eps []EndpointResult) Snapshot {
	return Snapshot{
		SiteResults:     site,
		DomainResults:   dom,
		EndpointResults: eps,
	}
}

func SetOfficialSiteResults(results []SiteResult) {
	Official.Mu.Lock()
	defer Official.Mu.Unlock()
	Official.SiteResults = cloneSiteResults(results)
}

func SetOfficialDomainResults(results []DomainResult) {
	Official.Mu.Lock()
	defer Official.Mu.Unlock()
	Official.DomainResults = cloneDomainResults(results)
}

func SetOfficialEndpointResults(results []EndpointResult) {
	Official.Mu.Lock()
	defer Official.Mu.Unlock()
	Official.EndpointResults = cloneEndpointResults(results)
}

func UpdateOfficialSiteResult(check cfg.Check, member cfg.Member, status bool, errorMsg string, dataMap map[string]interface{}, isIPv6 bool) {
	Official.Mu.Lock()
	var pendingEvent *pendingOfficialEvent

	sIndex := -1
	for i, sr := range Official.SiteResults {
		if sr.Check.Name == check.Name && sr.IsIPv6 == isIPv6 {
			sIndex = i
			break
		}
	}

	newResult := Result{
		Member:    cloneMember(member),
		Status:    status,
		Checktime: time.Now().UTC(),
		ErrorText: errorMsg,
		Data:      cloneAnyMap(dataMap),
		IsIPv6:    isIPv6,
	}

	if sIndex == -1 {
		Official.SiteResults = append(Official.SiteResults, SiteResult{
			Check:   cloneCheck(check),
			IsIPv6:  isIPv6,
			Results: []Result{newResult},
		})
		if !status {
			pendingEvent = &pendingOfficialEvent{
				checkType:  "site",
				checkName:  check.Name,
				memberName: member.Details.Name,
				status:     false,
				errorText:  errorMsg,
				data:       cloneAnyMap(dataMap),
				isIPv6:     isIPv6,
			}
		}
	} else {
		sr := &Official.SiteResults[sIndex]
		rIndex := -1
		for i, res := range sr.Results {
			if res.Member.Details.Name == member.Details.Name {
				rIndex = i
				break
			}
		}
		if rIndex == -1 {
			sr.Results = append(sr.Results, newResult)
			if !status {
				pendingEvent = &pendingOfficialEvent{
					checkType:  "site",
					checkName:  check.Name,
					memberName: member.Details.Name,
					status:     false,
					errorText:  errorMsg,
					data:       cloneAnyMap(dataMap),
					isIPv6:     isIPv6,
				}
			}
		} else {
			if sr.Results[rIndex].Status != status {
				pendingEvent = &pendingOfficialEvent{
					checkType:  "site",
					checkName:  check.Name,
					memberName: member.Details.Name,
					status:     status,
					errorText:  errorMsg,
					data:       cloneAnyMap(dataMap),
					isIPv6:     isIPv6,
				}
			}
			sr.Results[rIndex] = newResult
		}
	}

	publishSnapshotLocked()
	Official.Mu.Unlock()
	if pendingEvent != nil {
		go pendingEvent.emit()
	}
}

func UpdateOfficialDomainResult(check cfg.Check, member cfg.Member, service cfg.Service, domain string,
	status bool, errorMsg string, dataMap map[string]interface{}, isIPv6 bool) {

	Official.Mu.Lock()
	var pendingEvent *pendingOfficialEvent

	dIndex := -1
	for i, dr := range Official.DomainResults {
		if dr.Check.Name == check.Name && dr.Domain == domain && dr.IsIPv6 == isIPv6 {
			dIndex = i
			break
		}
	}

	newResult := Result{
		Member:    cloneMember(member),
		Status:    status,
		Checktime: time.Now().UTC(),
		ErrorText: errorMsg,
		Data:      cloneAnyMap(dataMap),
		IsIPv6:    isIPv6,
	}

	if dIndex == -1 {
		Official.DomainResults = append(Official.DomainResults, DomainResult{
			Check:   cloneCheck(check),
			Service: cloneService(service),
			Domain:  domain,
			IsIPv6:  isIPv6,
			Results: []Result{newResult},
		})
		if !status {
			pendingEvent = &pendingOfficialEvent{
				checkType:  "domain",
				checkName:  check.Name,
				memberName: member.Details.Name,
				domainName: domain,
				status:     false,
				errorText:  "Offline since startup",
				data:       cloneAnyMap(dataMap),
				isIPv6:     isIPv6,
			}
		}
	} else {
		dr := &Official.DomainResults[dIndex]
		rIndex := -1
		for i, res := range dr.Results {
			if res.Member.Details.Name == member.Details.Name {
				rIndex = i
				break
			}
		}
		if rIndex == -1 {
			dr.Results = append(dr.Results, newResult)
			if !status {
				pendingEvent = &pendingOfficialEvent{
					checkType:  "domain",
					checkName:  check.Name,
					memberName: member.Details.Name,
					domainName: domain,
					status:     false,
					errorText:  "Offline since startup",
					data:       cloneAnyMap(dataMap),
					isIPv6:     isIPv6,
				}
			}
		} else {
			if dr.Results[rIndex].Status != status {
				pendingEvent = &pendingOfficialEvent{
					checkType:  "domain",
					checkName:  check.Name,
					memberName: member.Details.Name,
					domainName: domain,
					status:     status,
					errorText:  errorMsg,
					data:       cloneAnyMap(dataMap),
					isIPv6:     isIPv6,
				}
			}
			dr.Results[rIndex] = newResult
		}
	}

	publishSnapshotLocked()
	Official.Mu.Unlock()
	if pendingEvent != nil {
		go pendingEvent.emit()
	}
}

func UpdateOfficialEndpointResult(check cfg.Check, member cfg.Member, service cfg.Service, domain string, endpoint string,
	status bool, errorMsg string, dataMap map[string]interface{}, isIPv6 bool) {

	Official.Mu.Lock()
	var pendingEvent *pendingOfficialEvent

	eIndex := -1
	for i, er := range Official.EndpointResults {
		if er.Check.Name == check.Name && er.Domain == domain && er.RpcUrl == endpoint && er.IsIPv6 == isIPv6 {
			eIndex = i
			break
		}
	}

	newResult := Result{
		Member:    cloneMember(member),
		Status:    status,
		Checktime: time.Now().UTC(),
		ErrorText: errorMsg,
		Data:      cloneAnyMap(dataMap),
		IsIPv6:    isIPv6,
	}

	if eIndex == -1 {
		Official.EndpointResults = append(Official.EndpointResults, EndpointResult{
			Check:   cloneCheck(check),
			Service: cloneService(service),
			RpcUrl:  endpoint,
			Domain:  domain,
			IsIPv6:  isIPv6,
			Results: []Result{newResult},
		})
		if !status {
			pendingEvent = &pendingOfficialEvent{
				checkType:  "endpoint",
				checkName:  check.Name,
				memberName: member.Details.Name,
				domainName: domain,
				endpoint:   endpoint,
				status:     false,
				errorText:  "Offline since startup",
				data:       cloneAnyMap(dataMap),
				isIPv6:     isIPv6,
			}
		}
	} else {
		er := &Official.EndpointResults[eIndex]
		rIndex := -1
		for i, res := range er.Results {
			if res.Member.Details.Name == member.Details.Name {
				rIndex = i
				break
			}
		}
		if rIndex == -1 {
			er.Results = append(er.Results, newResult)
			if !status {
				pendingEvent = &pendingOfficialEvent{
					checkType:  "endpoint",
					checkName:  check.Name,
					memberName: member.Details.Name,
					domainName: domain,
					endpoint:   endpoint,
					status:     false,
					errorText:  "Offline since startup",
					data:       cloneAnyMap(dataMap),
					isIPv6:     isIPv6,
				}
			}
		} else {
			if er.Results[rIndex].Status != status {
				pendingEvent = &pendingOfficialEvent{
					checkType:  "endpoint",
					checkName:  check.Name,
					memberName: member.Details.Name,
					domainName: domain,
					endpoint:   endpoint,
					status:     status,
					errorText:  errorMsg,
					data:       cloneAnyMap(dataMap),
					isIPv6:     isIPv6,
				}
			}
			er.Results[rIndex] = newResult
		}
	}

	publishSnapshotLocked()
	Official.Mu.Unlock()
	if pendingEvent != nil {
		go pendingEvent.emit()
	}
}

func latestStatusFromResults(results []Result, memberName string) (found bool, latest bool, newest time.Time) {
	for _, r := range results {
		if r.Member.Details.Name != memberName {
			continue
		}
		if !found || r.Checktime.After(newest) {
			found = true
			latest = r.Status
			newest = r.Checktime
		}
	}
	return
}

func GetOfficialSiteStatus(checkName, memberName string, isIPv6 bool) (bool, bool) {
	sites, _, _ := GetOfficialResults()

	var newest time.Time
	var latest bool
	found := false

	for _, sr := range sites {
		if sr.Check.Name != checkName || sr.IsIPv6 != isIPv6 {
			continue
		}
		if ok, st, ct := latestStatusFromResults(sr.Results, memberName); ok {
			if !found || ct.After(newest) {
				found = true
				latest = st
				newest = ct
			}
		}
	}
	return found, latest
}

func GetOfficialDomainStatus(checkName, memberName, domain string, isIPv6 bool) (bool, bool) {
	_, domains, _ := GetOfficialResults()

	var newest time.Time
	var latest bool
	found := false

	for _, dr := range domains {
		if dr.Check.Name != checkName || dr.Domain != domain || dr.IsIPv6 != isIPv6 {
			continue
		}
		if ok, st, ct := latestStatusFromResults(dr.Results, memberName); ok {
			if !found || ct.After(newest) {
				found = true
				latest = st
				newest = ct
			}
		}
	}
	return found, latest
}

func GetOfficialEndpointStatus(checkName, memberName, domain, endpoint string, isIPv6 bool) (bool, bool) {
	_, _, endpoints := GetOfficialResults()

	var newest time.Time
	var latest bool
	found := false

	for _, er := range endpoints {
		if er.Check.Name != checkName || er.Domain != domain || er.RpcUrl != endpoint || er.IsIPv6 != isIPv6 {
			continue
		}
		if ok, st, ct := latestStatusFromResults(er.Results, memberName); ok {
			if !found || ct.After(newest) {
				found = true
				latest = st
				newest = ct
			}
		}
	}
	return found, latest
}

func cloneAnyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}

	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = cloneInterfaceValue(v)
	}

	return dst
}

func cloneInterfaceValue(v interface{}) interface{} {
	switch typed := v.(type) {
	case map[string]interface{}:
		return cloneAnyMap(typed)
	case []interface{}:
		dst := make([]interface{}, len(typed))
		for i, item := range typed {
			dst[i] = cloneInterfaceValue(item)
		}
		return dst
	case []string:
		dst := make([]string, len(typed))
		copy(dst, typed)
		return dst
	default:
		return v
	}
}

func cloneStringSliceMap(src map[string][]string) map[string][]string {
	if src == nil {
		return nil
	}

	dst := make(map[string][]string, len(src))
	for k, v := range src {
		if v == nil {
			dst[k] = nil
			continue
		}
		cp := make([]string, len(v))
		copy(cp, v)
		dst[k] = cp
	}

	return dst
}

func cloneProviders(src map[string]cfg.ServiceProvider) map[string]cfg.ServiceProvider {
	if src == nil {
		return nil
	}

	dst := make(map[string]cfg.ServiceProvider, len(src))
	for k, v := range src {
		cp := cfg.ServiceProvider{}
		if v.RpcUrls != nil {
			cp.RpcUrls = make([]string, len(v.RpcUrls))
			copy(cp.RpcUrls, v.RpcUrls)
		}
		dst[k] = cp
	}

	return dst
}

func cloneCheck(src cfg.Check) cfg.Check {
	src.ExtraOptions = cloneAnyMap(src.ExtraOptions)
	return src
}

func cloneMember(src cfg.Member) cfg.Member {
	src.ServiceAssignments = cloneStringSliceMap(src.ServiceAssignments)
	return src
}

func cloneService(src cfg.Service) cfg.Service {
	src.Providers = cloneProviders(src.Providers)
	return src
}

func cloneResult(src Result) Result {
	src.Member = cloneMember(src.Member)
	src.Data = cloneAnyMap(src.Data)
	return src
}

func cloneSiteResults(src []SiteResult) []SiteResult {
	if src == nil {
		return nil
	}

	dst := make([]SiteResult, len(src))
	for i, item := range src {
		dst[i] = SiteResult{
			Check:  cloneCheck(item.Check),
			IsIPv6: item.IsIPv6,
		}
		if item.Results != nil {
			dst[i].Results = make([]Result, len(item.Results))
			for j, result := range item.Results {
				dst[i].Results[j] = cloneResult(result)
			}
		}
	}

	return dst
}

func cloneDomainResults(src []DomainResult) []DomainResult {
	if src == nil {
		return nil
	}

	dst := make([]DomainResult, len(src))
	for i, item := range src {
		dst[i] = DomainResult{
			Check:   cloneCheck(item.Check),
			Service: cloneService(item.Service),
			Domain:  item.Domain,
			IsIPv6:  item.IsIPv6,
		}
		if item.Results != nil {
			dst[i].Results = make([]Result, len(item.Results))
			for j, result := range item.Results {
				dst[i].Results[j] = cloneResult(result)
			}
		}
	}

	return dst
}

func cloneEndpointResults(src []EndpointResult) []EndpointResult {
	if src == nil {
		return nil
	}

	dst := make([]EndpointResult, len(src))
	for i, item := range src {
		dst[i] = EndpointResult{
			Check:    cloneCheck(item.Check),
			Service:  cloneService(item.Service),
			RpcUrl:   item.RpcUrl,
			Protocol: item.Protocol,
			Domain:   item.Domain,
			Port:     item.Port,
			Path:     item.Path,
			IsIPv6:   item.IsIPv6,
		}
		if item.Results != nil {
			dst[i].Results = make([]Result, len(item.Results))
			for j, result := range item.Results {
				dst[i].Results[j] = cloneResult(result)
			}
		}
	}

	return dst
}

func publishSnapshotLocked() {
	snap := BuildSnapshot(
		Official.SiteResults,
		Official.DomainResults,
		Official.EndpointResults,
	)
	SetOfficialSnapshot(snap)
}
