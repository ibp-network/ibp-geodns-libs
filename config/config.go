package config

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	log "github.com/ibp-network/ibp-geodns-libs/logging"
)

var (
	cfg *ConfigInit

	cfgInitMu      sync.Mutex
	configUpdaterC chan struct{}
	configClient   = &http.Client{Timeout: 15 * time.Second}
)

func Init(cfgFile string) {
	log.Log(log.Debug, "Config Package initializing...")

	cfgInitMu.Lock()
	defer cfgInitMu.Unlock()

	if cfg == nil {
		cfg = &ConfigInit{}
	}
	cfg.cfgFile = cfgFile

	if configUpdaterC != nil {
		close(configUpdaterC)
		configUpdaterC = nil
	}

	loadConfig(cfgFile, true)

	stop := make(chan struct{})
	configUpdaterC = stop
	go configUpdater(cfgFile, stop)
}

func loadConfig(cfgFile string, initialLoad bool) {
	if cfg == nil {
		return
	}

	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	loadSystemConfig(cfgFile, initialLoad)

	loadStaticDNSConfig(cfg.data.Local.System.ConfigUrls.StaticDNSConfig, initialLoad)
	loadMembersConfig(cfg.data.Local.System.ConfigUrls.MembersConfig, initialLoad)
	loadServicesConfig(cfg.data.Local.System.ConfigUrls.ServicesConfig, initialLoad)
	loadIaasPricing(cfg.data.Local.System.ConfigUrls.IaasPricingConfig, initialLoad)
	loadServiceRequestsConfig(cfg.data.Local.System.ConfigUrls.ServicesRequestsConfig, initialLoad)
	loadAlertsConfig(cfg.data.Local.System.ConfigUrls.AlertsConfig, initialLoad)
}

func loadAlertsConfig(url string, initialLoad bool) {
	if url == "" {
		// default to hardcoded URL if not specified
		url = "https://raw.githubusercontent.com/ibp-network/config/refs/heads/main/alerts.json"
	}

	data := downloadConfig(url, initialLoad)
	if data == nil {
		return
	}

	var alerts AlertsConfig
	if err := json.Unmarshal(data, &alerts); err != nil {
		log.Log(log.Error, "Failed to unmarshal Alerts config: %v", err)
		if initialLoad {
			log.Log(log.Fatal, "Terminating program due to critical error on initial load.")
			os.Exit(1)
		}
		return
	}

	cfg.data.Alerts = alerts
	log.Log(log.Debug, "Alerts configuration loaded from %s", url)
}

func loadSystemConfig(configPath string, initialLoad bool) {
	file, err := os.Open(configPath)
	if err != nil {
		log.Log(log.Error, "Failed to open system config file: %v", err)
		if initialLoad {
			log.Log(log.Fatal, "Terminating program due to critical error on initial load.")
			os.Exit(1)
		}
		return
	}
	defer file.Close()

	var systemConfig LocalConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&systemConfig); err != nil {
		log.Log(log.Error, "Failed to decode system config: %v", err)
		if initialLoad {
			log.Log(log.Fatal, "Terminating program due to critical error on initial load.")
			os.Exit(1)
		}
		return
	}

	cfg.data.Local = systemConfig
	log.Log(log.Debug, "System configuration loaded from %s", configPath)
}

func loadStaticDNSConfig(url string, initialLoad bool) {
	data := downloadConfig(url, initialLoad)
	if data == nil {
		return
	}

	var records []DNSRecord
	if err := json.Unmarshal(data, &records); err != nil {
		log.Log(log.Error, "Failed to unmarshal StaticDNS config: %v", err)
		if initialLoad {
			log.Log(log.Fatal, "Terminating program due to critical error on initial load.")
			os.Exit(1)
		}
		return
	}

	cfg.data.StaticDNS = records
	log.Log(log.Debug, "StaticDNS configuration loaded from %s", url)
}

func loadMembersConfig(url string, initialLoad bool) {
	data := downloadConfig(url, initialLoad)
	if data == nil {
		return
	}

	var newMembers map[string]Member
	if err := json.Unmarshal(data, &newMembers); err != nil {
		log.Log(log.Error, "Failed to unmarshal Members config: %v", err)
		if initialLoad {
			log.Log(log.Fatal, "Terminating program due to critical error on initial load.")
			os.Exit(1)
		}
		return
	}

	for name, existingMember := range cfg.data.Members {
		if existingMember.Override {
			if newMember, exists := newMembers[name]; exists {
				newMember.Override = true
				newMember.OverrideTime = existingMember.OverrideTime
				newMembers[name] = newMember
			}
		}
	}

	cfg.data.Members = newMembers
	log.Log(log.Debug, "Members configuration loaded from %s", url)
}

func loadServicesConfig(url string, initialLoad bool) {
	data := downloadConfig(url, initialLoad)
	if data == nil {
		return
	}
	var services map[string]Service
	if err := json.Unmarshal(data, &services); err != nil {
		log.Log(log.Error, "Failed to unmarshal Services config: %v", err)
		if initialLoad {
			log.Log(log.Fatal, "Terminating program due to critical error on initial load.")
			os.Exit(1)
		}
		return
	}

	cfg.data.Services = services
	log.Log(log.Debug, "Services configuration loaded from %s", url)
}

func loadIaasPricing(url string, initialLoad bool) {
	data := downloadConfig(url, initialLoad)
	if data == nil {
		return
	}

	var pricing map[string]IaasPricing
	if err := json.Unmarshal(data, &pricing); err != nil {
		log.Log(log.Error, "Failed to unmarshal IaaS pricing config: %v", err)
		if initialLoad {
			log.Log(log.Fatal, "Terminating program due to critical error on initial load.")
			os.Exit(1)
		}
		return
	}

	cfg.data.Pricing = pricing
	log.Log(log.Debug, "IaaS pricing configuration loaded from %s", url)
}

func loadServiceRequestsConfig(url string, initialLoad bool) {
	data := downloadConfig(url, initialLoad)
	if data == nil {
		return
	}
	var requests ServiceRequests
	if err := json.Unmarshal(data, &requests); err != nil {
		_, _, line, _ := runtime.Caller(2)
		log.Log(log.Error, "Failed to unmarshal ServiceRequests config Line: %d Error: %v", line, err)
		if initialLoad {
			log.Log(log.Fatal, "Terminating program due to critical error on initial load.")
			os.Exit(1)
		}
		return
	}

	cfg.data.ServiceRequests = requests
	log.Log(log.Debug, "Services configuration loaded from %s", url)
}

func downloadConfig(url string, initialLoad bool) []byte {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Log(log.Error, "Failed to create HTTP request for config from %s: %v", url, err)
		if initialLoad {
			_, _, line, _ := runtime.Caller(2)
			log.Log(log.Fatal, "Terminating program due to critical error on initial load. Line: %d", line)
			os.Exit(1)
		}
		return nil
	}

	resp, err := configClient.Do(req)
	if err != nil {
		log.Log(log.Error, "Failed to download config from %s: %v", url, err)
		if initialLoad {
			_, _, line, _ := runtime.Caller(2)
			log.Log(log.Fatal, "Terminating program due to critical error on initial load. Line: %d", line)
			os.Exit(1)
		}
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Log(log.Error, "Non-OK HTTP status while downloading config from %s: %s", url, resp.Status)
		if initialLoad {
			_, _, line, _ := runtime.Caller(2)
			log.Log(log.Fatal, "Terminating program due to critical error on initial load. Line: %d", line)
			os.Exit(1)
		}
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Log(log.Error, "Failed to read response body from %s: %v", url, err)
		if initialLoad {
			_, _, line, _ := runtime.Caller(2)
			log.Log(log.Fatal, "Terminating program due to critical error on initial load. Line: %d", line)
			os.Exit(1)
		}
		return nil
	}

	return data
}

func configUpdater(cfgFile string, stop <-chan struct{}) {
	for {
		c := GetConfig()
		interval := c.Local.System.ConfigReloadTime
		if interval <= 0 {
			log.Log(log.Warn, "ConfigReloadTime <= 0; skipping reload, retrying in 30s")
			interval = 30
		}

		timer := time.NewTimer(interval * time.Second)
		select {
		case <-stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
			loadConfig(cfgFile, false)
		}
	}
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

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}

	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}

	return dst
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

func cloneChecks(src []Check) []Check {
	if src == nil {
		return nil
	}

	dst := make([]Check, len(src))
	for i, check := range src {
		dst[i] = check
		dst[i].ExtraOptions = cloneAnyMap(check.ExtraOptions)
	}

	return dst
}

func cloneLocalConfig(src LocalConfig) LocalConfig {
	dst := src
	dst.DnsApi.AuthKeys = cloneStringMap(src.DnsApi.AuthKeys)
	dst.CollatorApi.AuthKeys = cloneStringMap(src.CollatorApi.AuthKeys)
	dst.MonitorApi.AuthKeys = cloneStringMap(src.MonitorApi.AuthKeys)
	dst.MgmtApi.AuthKeys = cloneStringMap(src.MgmtApi.AuthKeys)
	dst.Checks = cloneChecks(src.Checks)
	return dst
}

func cloneMember(src Member) Member {
	dst := src
	dst.ServiceAssignments = cloneStringSliceMap(src.ServiceAssignments)
	return dst
}

func cloneMembers(src map[string]Member) map[string]Member {
	if src == nil {
		return nil
	}

	dst := make(map[string]Member, len(src))
	for k, member := range src {
		dst[k] = cloneMember(member)
	}

	return dst
}

func cloneServiceProviders(src map[string]ServiceProvider) map[string]ServiceProvider {
	if src == nil {
		return nil
	}

	dst := make(map[string]ServiceProvider, len(src))
	for k, provider := range src {
		cp := ServiceProvider{}
		if provider.RpcUrls != nil {
			cp.RpcUrls = make([]string, len(provider.RpcUrls))
			copy(cp.RpcUrls, provider.RpcUrls)
		}
		dst[k] = cp
	}

	return dst
}

func cloneServices(src map[string]Service) map[string]Service {
	if src == nil {
		return nil
	}

	dst := make(map[string]Service, len(src))
	for k, service := range src {
		cp := service
		cp.Providers = cloneServiceProviders(service.Providers)
		dst[k] = cp
	}

	return dst
}

func clonePricing(src map[string]IaasPricing) map[string]IaasPricing {
	if src == nil {
		return nil
	}

	dst := make(map[string]IaasPricing, len(src))
	for k, pricing := range src {
		dst[k] = pricing
	}

	return dst
}

func cloneServiceRequests(src ServiceRequests) ServiceRequests {
	dst := ServiceRequests{}
	if src.Requests == nil {
		return dst
	}

	dst.Requests = make(map[string]map[string]MonthlyData, len(src.Requests))
	for serviceName, monthly := range src.Requests {
		if monthly == nil {
			dst.Requests[serviceName] = nil
			continue
		}

		monthlyCopy := make(map[string]MonthlyData, len(monthly))
		for month, data := range monthly {
			monthlyCopy[month] = data
		}
		dst.Requests[serviceName] = monthlyCopy
	}

	return dst
}

func cloneAlertsConfig(src AlertsConfig) AlertsConfig {
	dst := src
	dst.Matrix.Members = cloneStringSliceMap(src.Matrix.Members)
	return dst
}

func cloneConfigData(src Config) Config {
	dst := src
	dst.Local = cloneLocalConfig(src.Local)
	if src.StaticDNS != nil {
		dst.StaticDNS = make([]DNSRecord, len(src.StaticDNS))
		copy(dst.StaticDNS, src.StaticDNS)
	}
	dst.Members = cloneMembers(src.Members)
	dst.Services = cloneServices(src.Services)
	dst.Pricing = clonePricing(src.Pricing)
	dst.ServiceRequests = cloneServiceRequests(src.ServiceRequests)
	dst.Alerts = cloneAlertsConfig(src.Alerts)
	return dst
}

func GetConfig() Config {
	if cfg == nil {
		return Config{}
	}

	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	return cloneConfigData(cfg.data)
}
