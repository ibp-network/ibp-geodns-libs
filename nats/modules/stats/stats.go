package stats

import (
	"encoding/json"
	"fmt"
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
	CountActiveMonitors func() int
	MarkNodeHeard       func(string)
	StatsDataSubject    string
}

func HandleRequest(deps Dependencies, reply string, data []byte) {
	var req core.DowntimeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Log(log.Error, "[NATS] handleMonitorStatsRequest: unmarshal error: %v", err)
		if reply != "" {
			errResp := core.DowntimeResponse{
				NodeID: deps.State.NodeID,
				Events: []core.DowntimeEvent{},
				Error:  fmt.Sprintf("unmarshal error: %v", err),
			}
			if payload, err := json.Marshal(errResp); err == nil {
				_ = deps.PublishMsgWithReply(reply, "", payload)
			}
		}
		return
	}

	log.Log(log.Debug, "[NATS] handleMonitorStatsRequest: StartTime=%v EndTime=%v MemberName=%s",
		req.StartTime, req.EndTime, req.MemberName)

	if req.EndTime.Before(req.StartTime) {
		log.Log(log.Error, "[NATS] handleMonitorStatsRequest: EndTime before StartTime")
		if reply != "" {
			errResp := core.DowntimeResponse{
				NodeID: deps.State.NodeID,
				Events: []core.DowntimeEvent{},
				Error:  "EndTime must be after StartTime",
			}
			if payload, err := json.Marshal(errResp); err == nil {
				_ = deps.PublishMsgWithReply(reply, "", payload)
			}
		}
		return
	}

	events, err := retrieveLocalDowntimeEvents(req.MemberName, req.StartTime, req.EndTime)
	if err != nil {
		log.Log(log.Error, "[NATS] handleMonitorStatsRequest: error retrieving local downtime: %v", err)
		events = []core.DowntimeEvent{}
	}

	resp := core.DowntimeResponse{
		NodeID: deps.State.NodeID,
		Events: events,
	}
	payload, _ := json.Marshal(resp)

	if reply != "" {
		log.Log(log.Debug,
			"[NATS] handleMonitorStatsRequest: replying to %s with %d events",
			reply, len(events))
		_ = deps.PublishMsgWithReply(reply, "", payload)
	} else if deps.StatsDataSubject != "" {
		log.Log(log.Debug,
			"[NATS] handleMonitorStatsRequest: publishing downtimeData with %d events",
			len(events))
		_ = deps.Publish(deps.StatsDataSubject, payload)
	}
}

func HandleData(deps Dependencies, data []byte) {
	var resp core.DowntimeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		log.Log(log.Error, "[NATS] handleMonitorStatsData: unmarshal error: %v", err)
		return
	}
	if deps.MarkNodeHeard != nil {
		deps.MarkNodeHeard(resp.NodeID)
	}
	log.Log(log.Debug, "[NATS] handleMonitorStatsData: got %d downtime events from node=%s",
		len(resp.Events), resp.NodeID)
}

func RequestAll(deps Dependencies, req core.DowntimeRequest, timeout time.Duration, subject string) ([]core.DowntimeEvent, error) {
	monitorCount := deps.CountActiveMonitors()
	if monitorCount == 0 {
		return nil, fmt.Errorf("no active IBPMonitor nodes found")
	}

	log.Log(log.Debug, "[NATS] RequestAllMonitorsDowntime: requesting from %d active monitors", monitorCount)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("downtime request marshal error: %w", err)
	}

	inbox := fmt.Sprintf("_INBOX.%s.downtimeReply.%d", deps.State.NodeID, time.Now().UnixNano())
	responseMap := make(map[string][]core.DowntimeEvent)
	var mu sync.Mutex

	sub, err := deps.Subscribe(inbox, func(msg *nats.Msg) {
		var resp core.DowntimeResponse
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			log.Log(log.Error, "[NATS] RequestAllMonitorsDowntime: unmarshal error: %v", err)
			return
		}

		mu.Lock()
		if _, exists := responseMap[resp.NodeID]; !exists {
			responseMap[resp.NodeID] = resp.Events
			log.Log(log.Debug, "[NATS] RequestAllMonitorsDowntime: received %d events from %s",
				len(resp.Events), resp.NodeID)
		} else {
			log.Log(log.Warn, "[NATS] RequestAllMonitorsDowntime: duplicate response from %s ignored", resp.NodeID)
		}
		mu.Unlock()
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe error: %w", err)
	}
	defer sub.Unsubscribe()

	if err := deps.PublishMsgWithReply(subject, inbox, payload); err != nil {
		return nil, fmt.Errorf("publish downtime request error: %w", err)
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
				"[NATS] RequestAllMonitorsDowntime: timeout after receiving %d/%d responses",
				receivedCount, monitorCount)
			goto done
		case <-ticker.C:
			mu.Lock()
			if len(responseMap) >= monitorCount {
				mu.Unlock()
				log.Log(log.Debug, "[NATS] RequestAllMonitorsDowntime: received all %d responses", monitorCount)
				goto done
			}
			mu.Unlock()
		}
	}

done:
	mu.Lock()
	defer mu.Unlock()

	aggregated := make([]core.DowntimeEvent, 0)
	for nodeID, events := range responseMap {
		log.Log(log.Debug, "[NATS] RequestAllMonitorsDowntime: aggregating %d events from %s",
			len(events), nodeID)
		aggregated = append(aggregated, events...)
	}

	log.Log(log.Debug,
		"[NATS] RequestAllMonitorsDowntime: completed with %d total events from %d nodes",
		len(aggregated), len(responseMap))

	return aggregated, nil
}

func retrieveLocalDowntimeEvents(memberName string, start, end time.Time) ([]core.DowntimeEvent, error) {
	log.Log(log.Debug,
		"[NATS] retrieveLocalDowntimeEvents: memberName=%s start=%v end=%v",
		memberName, start, end)

	rawEvents, err := dat.GetMemberEvents(memberName, "", start, end)
	if err != nil {
		return nil, err
	}

	results := make([]core.DowntimeEvent, 0, len(rawEvents))
	for _, e := range rawEvents {
		if !e.Status {
			results = append(results, core.DowntimeEvent{
				MemberName: e.MemberName,
				CheckType:  e.CheckType,
				CheckName:  e.CheckName,
				DomainName: e.DomainName,
				Endpoint:   e.Endpoint,
				Status:     e.Status,
				StartTime:  e.StartTime,
				EndTime:    e.EndTime,
				ErrorText:  e.ErrorText,
				Data:       e.Data,
				IsIPv6:     e.IsIPv6,
			})
		}
	}

	log.Log(log.Debug,
		"[NATS] retrieveLocalDowntimeEvents: found %d total events, returning %d downtime events for member=%s",
		len(rawEvents), len(results), memberName)

	return results, nil
}
