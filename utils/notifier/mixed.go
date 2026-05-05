package notifier

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	messageevent "github.com/komari-monitor/komari/database/models/messageEvent"
	recordsdb "github.com/komari-monitor/komari/database/records"
	"github.com/komari-monitor/komari/utils/messageSender"
)

type MixedNotificationService struct {
	mu       sync.Mutex
	tickers  map[int]*time.Ticker
	tasks    map[int][]models.MixedNotification
	stopChan chan struct{}
}

type mixedUsageSnapshot struct {
	Cpu     float32
	Ram     float32
	Load    float32
	Traffic float32
}

type mixedClientAlert struct {
	Client  models.Client
	Message string
}

var MixedNotificationManager = &MixedNotificationService{
	tickers:  make(map[int]*time.Ticker),
	tasks:    make(map[int][]models.MixedNotification),
	stopChan: make(chan struct{}),
}

func (m *MixedNotificationService) Reload(rules []models.MixedNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopChan != nil {
		close(m.stopChan)
	}
	for _, ticker := range m.tickers {
		ticker.Stop()
	}

	m.stopChan = make(chan struct{})
	m.tickers = make(map[int]*time.Ticker)
	m.tasks = make(map[int][]models.MixedNotification)

	taskGroups := make(map[int][]models.MixedNotification)
	for _, rule := range rules {
		taskGroups[rule.Interval] = append(taskGroups[rule.Interval], rule)
	}

	for interval, groupedRules := range taskGroups {
		ticker := time.NewTicker(time.Duration(interval) * time.Minute)
		stopChan := m.stopChan
		m.tickers[interval] = ticker
		m.tasks[interval] = groupedRules

		go func(ticker *time.Ticker, tasks []models.MixedNotification, stop <-chan struct{}) {
			for {
				select {
				case <-ticker.C:
					for _, task := range tasks {
						go executeMixedNotificationTask(task)
					}
				case <-stop:
					return
				}
			}
		}(ticker, groupedRules, stopChan)
	}

	return nil
}

func executeMixedNotificationTask(rule models.MixedNotification) {
	if shouldSkipMixedNotification(rule) {
		return
	}

	now := time.Now()
	windowStart := now.Add(-time.Duration(rule.Interval) * time.Minute)
	alerts := make([]mixedClientAlert, 0)

	for _, clientUUID := range rule.Clients {
		client, err := clients.GetClientByUUID(clientUUID)
		if err != nil {
			continue
		}

		clientRecords, err := recordsdb.GetRecordsByClientAndTime(clientUUID, windowStart, now)
		if err != nil || len(clientRecords) == 0 {
			continue
		}

		if matched, detail := checkMixedThreshold(clientRecords, client, rule); matched {
			alerts = append(alerts, mixedClientAlert{
				Client:  client,
				Message: detail,
			})
		}
	}

	if sendMixedNotification(alerts, rule, now) {
		updateMixedLastNotified(rule.Id, now)
	}
}

func shouldSkipMixedNotification(rule models.MixedNotification) bool {
	if rule.LastNotified.ToTime().IsZero() {
		return false
	}

	cooldownPeriod := time.Duration(rule.Interval) * time.Minute
	return time.Since(rule.LastNotified.ToTime()) < cooldownPeriod
}

func checkMixedThreshold(records []models.Record, client models.Client, rule models.MixedNotification) (bool, string) {
	if len(records) == 0 {
		return false, ""
	}

	activeMetrics := activeMixedMetricCount(rule)
	if activeMetrics == 0 {
		return false, ""
	}

	requiredMatches := rule.MatchCount
	if requiredMatches <= 0 {
		requiredMatches = 1
	}
	if requiredMatches > activeMetrics {
		requiredMatches = activeMetrics
	}

	requiredRecords := requiredMixedRecords(len(records), rule.Ratio)
	matchedRecords := 0
	peak := mixedUsageSnapshot{}

	for _, record := range records {
		exceededCount, snapshot := calculateMixedExceedCount(record, client, rule)
		if exceededCount >= requiredMatches {
			matchedRecords++
			peak = mergeMixedPeak(peak, snapshot)
		}
	}

	if matchedRecords < requiredRecords {
		return false, ""
	}

	return true, buildMixedAlertDetail(client, rule, matchedRecords, len(records), requiredRecords, peak)
}

func activeMixedMetricCount(rule models.MixedNotification) int {
	count := 0
	if rule.CpuThreshold > 0 {
		count++
	}
	if rule.RamThreshold > 0 {
		count++
	}
	if rule.LoadThreshold > 0 {
		count++
	}
	if rule.TrafficThreshold > 0 {
		count++
	}
	return count
}

func requiredMixedRecords(total int, ratio float32) int {
	if total <= 0 {
		return 0
	}
	if ratio <= 0 {
		return 1
	}

	normalizedRatio := math.Round(float64(ratio)*10000) / 10000
	requiredFloat := float64(total) * normalizedRatio
	required := int(math.Ceil(requiredFloat - 1e-9))
	if required < 1 {
		required = 1
	}
	if required > total {
		required = total
	}
	return required
}

func calculateMixedExceedCount(record models.Record, client models.Client, rule models.MixedNotification) (int, mixedUsageSnapshot) {
	snapshot := mixedUsageSnapshot{
		Cpu:     record.Cpu,
		Load:    record.Load,
		Traffic: bytesPerSecondToMbps(record.NetIn + record.NetOut),
	}
	if client.MemTotal > 0 {
		snapshot.Ram = float32(record.Ram) / float32(client.MemTotal) * 100
	}

	count := 0
	if rule.CpuThreshold > 0 && snapshot.Cpu >= rule.CpuThreshold {
		count++
	}
	if rule.RamThreshold > 0 && snapshot.Ram >= rule.RamThreshold {
		count++
	}
	if rule.LoadThreshold > 0 && snapshot.Load >= rule.LoadThreshold {
		count++
	}
	if rule.TrafficThreshold > 0 && snapshot.Traffic >= rule.TrafficThreshold {
		count++
	}

	return count, snapshot
}

func mergeMixedPeak(current, candidate mixedUsageSnapshot) mixedUsageSnapshot {
	if candidate.Cpu > current.Cpu {
		current.Cpu = candidate.Cpu
	}
	if candidate.Ram > current.Ram {
		current.Ram = candidate.Ram
	}
	if candidate.Load > current.Load {
		current.Load = candidate.Load
	}
	if candidate.Traffic > current.Traffic {
		current.Traffic = candidate.Traffic
	}
	return current
}

func buildMixedAlertDetail(client models.Client, rule models.MixedNotification, matchedRecords, totalRecords, requiredRecords int, peak mixedUsageSnapshot) string {
	lines := []string{
		fmt.Sprintf("◈ %s", mixedClientName(client)),
		fmt.Sprintf("命中记录：%d/%d（需至少 %d 条）", matchedRecords, totalRecords, requiredRecords),
	}

	if thresholds := formatMixedThresholds(rule); thresholds != "" {
		lines = append(lines, "触发条件："+thresholds)
	}
	if peaks := formatMixedPeaks(rule, peak); peaks != "" {
		lines = append(lines, "峰值概览："+peaks)
	}

	return strings.Join(lines, "\n")
}

func formatMixedThresholds(rule models.MixedNotification) string {
	parts := make([]string, 0, 4)
	if rule.CpuThreshold > 0 {
		parts = append(parts, fmt.Sprintf("CPU >= %.1f%%", rule.CpuThreshold))
	}
	if rule.RamThreshold > 0 {
		parts = append(parts, fmt.Sprintf("内存 >= %.1f%%", rule.RamThreshold))
	}
	if rule.LoadThreshold > 0 {
		parts = append(parts, fmt.Sprintf("负载 >= %.1f", rule.LoadThreshold))
	}
	if rule.TrafficThreshold > 0 {
		parts = append(parts, fmt.Sprintf("流量 >= %.1f Mbps", rule.TrafficThreshold))
	}
	return strings.Join(parts, " ｜ ")
}

func formatMixedPeaks(rule models.MixedNotification, peak mixedUsageSnapshot) string {
	parts := make([]string, 0, 4)
	if rule.CpuThreshold > 0 {
		parts = append(parts, fmt.Sprintf("CPU %.1f%%", peak.Cpu))
	}
	if rule.RamThreshold > 0 {
		parts = append(parts, fmt.Sprintf("内存 %.1f%%", peak.Ram))
	}
	if rule.LoadThreshold > 0 {
		parts = append(parts, fmt.Sprintf("负载 %.1f", peak.Load))
	}
	if rule.TrafficThreshold > 0 {
		parts = append(parts, fmt.Sprintf("流量 %.1f Mbps", peak.Traffic))
	}
	return strings.Join(parts, " ｜ ")
}

func mixedClientName(client models.Client) string {
	if strings.TrimSpace(client.Name) != "" {
		return client.Name
	}
	return client.UUID
}

func sendMixedNotification(alerts []mixedClientAlert, rule models.MixedNotification, notifyTime time.Time) bool {
	if len(alerts) == 0 {
		return false
	}

	sort.Slice(alerts, func(i, j int) bool {
		return mixedClientName(alerts[i].Client) < mixedClientName(alerts[j].Client)
	})

	clientsForMessage := make([]models.Client, 0, len(alerts))
	messageLines := []string{
		fmt.Sprintf("混合告警规则：%s", strings.TrimSpace(rule.Name)),
		fmt.Sprintf("监测窗口：最近 %d 分钟，至少 %d 项指标同时异常", rule.Interval, maxInt(rule.MatchCount, 1)),
		"",
	}

	for _, alert := range alerts {
		clientsForMessage = append(clientsForMessage, alert.Client)
		messageLines = append(messageLines, alert.Message, "")
	}

	err := messageSender.SendEvent(models.EventMessage{
		Event:   messageevent.Alert,
		Clients: clientsForMessage,
		Time:    notifyTime,
		Emoji:   "⚠️",
		Message: strings.TrimSpace(strings.Join(messageLines, "\n")),
	})
	if err != nil {
		log.Printf("Failed to send mixed notification: %v", err)
		return false
	}
	return true
}

func updateMixedLastNotified(taskID uint, notifyTime time.Time) {
	db := dbcore.GetDBInstance()
	if err := db.Model(&models.MixedNotification{}).Where("id = ?", taskID).Update("last_notified", notifyTime).Error; err != nil {
		log.Printf("Failed to update mixed notification %d: %v", taskID, err)
	}
}

func ReloadMixedNotificationSchedule(rules []models.MixedNotification) error {
	return MixedNotificationManager.Reload(rules)
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
