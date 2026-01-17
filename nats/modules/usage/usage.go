package usage

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	dat "github.com/ibp-network/ibp-geodns-libs/data"
	log "github.com/ibp-network/ibp-geodns-libs/logging"
	"github.com/ibp-network/ibp-geodns-libs/nats/core"

	"github.com/nats-io/nats.go"
)

type Dependencies struct {
	State               *core.NodeState
	Publish             func(subject string, data []byte) error
	PublishMsgWithReply func(subject, reply string, data []byte) error
	Subscribe           func(subject string, cb func(*nats.Msg)) (*nats.Subscription, error)
	CountActiveDns      func() int
	MarkNodeHeard       func(string)
	UsageDataSubject    string
}

func HandleRequest(deps Dependencies, reply string, data []byte) {
	var req core.UsageRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Log(log.Error, "[NATS] handleDnsUsageRequest: unmarshal error: %v", err)
		if reply != "" {
			errResp := core.UsageResponse{
				NodeID:       deps.State.NodeID,
				UsageRecords: []core.UsageRecord{},
				Error:        fmt.Sprintf("unmarshal error: %v", err),
			}
			if payload, err := json.Marshal(errResp); err == nil {
				_ = deps.PublishMsgWithReply(reply, "", payload)
			}
		}
		return
	}

	log.Log(log.Debug,
		"[NATS] handleDnsUsageRequest: StartDate=%s EndDate=%s Domain=%s MemberName=%s Country=%s",
		req.StartDate, req.EndDate, req.Domain, req.MemberName, req.Country)

	if req.StartDate > req.EndDate {
		log.Log(log.Error, "[NATS] handleDnsUsageRequest: StartDate after EndDate")
		if reply != "" {
			errResp := core.UsageResponse{
				NodeID:       deps.State.NodeID,
				UsageRecords: []core.UsageRecord{},
				Error:        "StartDate must be before or equal to EndDate",
			}
			if payload, err := json.Marshal(errResp); err == nil {
				_ = deps.PublishMsgWithReply(reply, "", payload)
			}
		}
		return
	}

	records, err := retrieveLocalUsageRecords(req.StartDate, req.EndDate, req.Domain, req.MemberName, req.Country)
	if err != nil {
		log.Log(log.Error,
			"[NATS] handleDnsUsageRequest: retrieveLocalUsageRecords error: %v",
			err)
		records = []core.UsageRecord{}
	}

	resp := core.UsageResponse{
		NodeID:       deps.State.NodeID,
		UsageRecords: records,
	}
	payload, _ := json.Marshal(resp)

	if reply != "" {
		log.Log(log.Debug,
			"[NATS] handleDnsUsageRequest: replying to %s with %d usage records",
			reply, len(records))
		_ = deps.PublishMsgWithReply(reply, "", payload)
	} else {
		if deps.UsageDataSubject != "" {
			log.Log(log.Debug,
				"[NATS] handleDnsUsageRequest: publishing usageData with %d usage records",
				len(records))
			_ = deps.Publish(deps.UsageDataSubject, payload)
		}
	}
}

func HandleData(deps Dependencies, data []byte) {
	var resp core.UsageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Log(log.Error, "[NATS] handleDnsUsageData: unmarshal error: %v", err)
		return
	}
	if deps.MarkNodeHeard != nil {
		deps.MarkNodeHeard(resp.NodeID)
	}

	log.Log(log.Debug, "[NATS] handleDnsUsageData: got %d usage records from node=%s",
		len(resp.UsageRecords), resp.NodeID)
}

func RequestAll(deps Dependencies, req core.UsageRequest, timeout time.Duration, subject string) ([]core.UsageRecord, error) {
	dnsCount := deps.CountActiveDns()
	if dnsCount == 0 {
		return nil, fmt.Errorf("no active IBPDns nodes found")
	}

	log.Log(log.Debug, "[NATS] RequestAllDnsUsage: requesting from %d active DNS nodes", dnsCount)

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("usage request marshal error: %w", err)
	}

	inbox := fmt.Sprintf("_INBOX.%s.usageReply.%d", deps.State.NodeID, time.Now().UnixNano())
	responseMap := make(map[string][]core.UsageRecord)
	var mu sync.Mutex

	sub, err := deps.Subscribe(inbox, func(msg *nats.Msg) {
		var resp core.UsageResponse
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			log.Log(log.Error, "[NATS] RequestAllDnsUsage: unmarshal error: %v", err)
			return
		}

		mu.Lock()
		if _, exists := responseMap[resp.NodeID]; !exists {
			responseMap[resp.NodeID] = resp.UsageRecords
			log.Log(log.Debug, "[NATS] RequestAllDnsUsage: received %d records from %s",
				len(resp.UsageRecords), resp.NodeID)
		} else {
			log.Log(log.Warn, "[NATS] RequestAllDnsUsage: duplicate response from %s ignored", resp.NodeID)
		}
		mu.Unlock()
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe error: %w", err)
	}
	defer sub.Unsubscribe()

	if err := deps.PublishMsgWithReply(subject, inbox, data); err != nil {
		return nil, fmt.Errorf("publish usage request error: %w", err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			mu.Lock()
			receivedCount := len(responseMap)
			mu.Unlock()
			log.Log(log.Warn,
				"[NATS] RequestAllDnsUsage: timeout after receiving %d/%d responses",
				receivedCount, dnsCount)
			goto done

		case <-ticker.C:
			mu.Lock()
			if len(responseMap) >= dnsCount {
				mu.Unlock()
				log.Log(log.Debug, "[NATS] RequestAllDnsUsage: received all %d responses", dnsCount)
				goto done
			}
			mu.Unlock()
		}
	}

done:
	mu.Lock()
	defer mu.Unlock()

	aggregateMap := make(map[string]core.UsageRecord)

	for nodeID, records := range responseMap {
		log.Log(log.Debug, "[NATS] RequestAllDnsUsage: aggregating %d records from %s",
			len(records), nodeID)
		for _, rec := range records {
			key := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
				rec.Date, rec.Domain, rec.MemberName, rec.CountryCode,
				rec.Asn, rec.NetworkName, rec.CountryName)

			if existing, found := aggregateMap[key]; found {
				existing.Hits += rec.Hits
				aggregateMap[key] = existing
			} else {
				aggregateMap[key] = rec
			}
		}
	}

	aggregated := make([]core.UsageRecord, 0, len(aggregateMap))
	for _, rec := range aggregateMap {
		aggregated = append(aggregated, rec)
	}

	log.Log(log.Debug,
		"[NATS] RequestAllDnsUsage: completed with %d unique records from %d nodes",
		len(aggregated), len(responseMap))

	return aggregated, nil
}

func retrieveLocalUsageRecords(
	startDate, endDate, domain, member, country string,
) ([]core.UsageRecord, error) {
	log.Log(log.Debug,
		"[NATS] retrieveLocalUsageRecords: start=%s end=%s domain=%s member=%s country=%s",
		startDate, endDate, domain, member, country)

	sd := strings.TrimSpace(startDate)
	ed := strings.TrimSpace(endDate)
	if len(sd) != 10 || len(ed) != 10 {
		return nil, fmt.Errorf("invalid date format, expected YYYY-MM-DD")
	}

	sTime, err := time.Parse("2006-01-02", sd)
	if err != nil {
		return nil, fmt.Errorf("invalid start date: %w", err)
	}
	eTime, err := time.Parse("2006-01-02", ed)
	if err != nil {
		return nil, fmt.Errorf("invalid end date: %w", err)
	}

	var results []core.UsageRecord

	if domain != "" && member != "" {
		recs, err := dat.GetUsageByMember(domain, member, sTime, eTime)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			if country == "" || strings.EqualFold(country, r.CountryCode) {
				results = append(results, core.UsageRecord{
					Date:        r.Date,
					Domain:      r.Domain,
					MemberName:  r.MemberName,
					CountryCode: r.CountryCode,
					Asn:         r.Asn,
					NetworkName: r.NetworkName,
					CountryName: r.CountryName,
					Hits:        r.Hits,
				})
			}
		}
	} else if domain != "" {
		recs, err := dat.GetUsageByDomain(domain, sTime, eTime)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			if country == "" || strings.EqualFold(country, r.CountryCode) {
				results = append(results, core.UsageRecord{
					Date:        r.Date,
					Domain:      r.Domain,
					MemberName:  r.MemberName,
					CountryCode: r.CountryCode,
					Asn:         r.Asn,
					NetworkName: r.NetworkName,
					CountryName: r.CountryName,
					Hits:        r.Hits,
				})
			}
		}
	} else {
		recs, err := dat.GetUsageByCountry(sTime, eTime)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			if member != "" && r.MemberName != member {
				continue
			}
			if country != "" && !strings.EqualFold(country, r.CountryCode) {
				continue
			}
			results = append(results, core.UsageRecord{
				Date:        r.Date,
				Domain:      r.Domain,
				MemberName:  r.MemberName,
				CountryCode: r.CountryCode,
				Asn:         r.Asn,
				NetworkName: r.NetworkName,
				CountryName: r.CountryName,
				Hits:        r.Hits,
			})
		}
	}

	log.Log(log.Debug,
		"[NATS] retrieveLocalUsageRecords: returning %d usage records",
		len(results))
	return results, nil
}
